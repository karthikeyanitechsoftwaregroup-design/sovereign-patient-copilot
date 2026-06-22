package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"sovereign/internal/agents"
	"sovereign/internal/models"
	pdfpkg "sovereign/pkg/pdf"
)

// NewRouter wires all routes and returns a ready *gin.Engine.
func NewRouter(db *pgxpool.Pool) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Type"},
		AllowCredentials: true,
	}))

	api := r.Group("/api")
	{
		// Health check
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
		})

		// Upload PDF → triggers two-agent pipeline, streams SSE back
		api.POST("/upload", uploadAndStream(db))

		// List all stored records
		api.GET("/records", listRecords(db))

		// Get one record + its suggestions
		api.GET("/records/:id", getRecord(db))

		// Mock EMR post (bonus MCP-style endpoint)
		api.POST("/emr/post/:id", postToEMR(db))
	}

	return r
}

// ─── POST /api/upload ────────────────────────────────────────────────────────
// Accepts multipart PDF, streams SSE events:
//   event: pipeline_stage  (data: {stage: 1|2|3})
//   event: scribe_done     (data: {summary: "..."})
//   event: suggestions     (data: [{title,description,priority,citation}, ...])
//   event: record_id       (data: {id: 42})
//   event: error           (data: {message: "..."})

func uploadAndStream(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse multipart form
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file field required"})
			return
		}
		defer file.Close()

		pdfBytes := make([]byte, header.Size)
		if _, err := file.Read(pdfBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "read file"})
			return
		}

		// Set SSE headers
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		send := func(eventType string, payload any) {
			data, _ := json.Marshal(payload)
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, data)
			c.Writer.Flush()
		}

		ctx := context.Background()

		// ── Stage 1: Scribe ──
		send("pipeline_stage", gin.H{"stage": 1, "label": "Agent A: Scribe extracting & de-identifying"})

		pdfText, err := pdfpkg.Extract(pdfBytes)
		if err != nil {
			send("error", gin.H{"message": fmt.Sprintf("PDF extraction: %v", err)})
			return
		}

		scribeSummary, err := agents.RunScribe(ctx, pdfText)
		if err != nil {
			send("error", gin.H{"message": fmt.Sprintf("Scribe agent: %v", err)})
			return
		}
		send("scribe_done", gin.H{"summary": scribeSummary})

		// ── Stage 2: Specialist ──
		send("pipeline_stage", gin.H{"stage": 2, "label": "Agent B: Specialist generating actions"})

		suggestions, err := agents.RunSpecialist(ctx, scribeSummary)
		if err != nil {
			send("error", gin.H{"message": fmt.Sprintf("Specialist agent: %v", err)})
			return
		}
		send("suggestions", suggestions)

		// ── Stage 3: Persist to Postgres ──
		send("pipeline_stage", gin.H{"stage": 3, "label": "Saving to records"})

		recordID, err := persistRecord(ctx, db, header.Filename, pdfText, scribeSummary, suggestions)
		if err != nil {
			log.Printf("persist error: %v", err)
			// non-fatal — we already sent suggestions
		}
		send("record_id", gin.H{"id": recordID})
		send("done", gin.H{"stage": 4})
	}
}

func persistRecord(
	ctx context.Context,
	db *pgxpool.Pool,
	filename, rawText, scribeSummary string,
	suggestions []agents.Suggestion,
) (int64, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var recordID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO patient_records (filename, raw_text, scribe_summary)
		 VALUES ($1, $2, $3) RETURNING id`,
		filename, rawText, scribeSummary,
	).Scan(&recordID)
	if err != nil {
		return 0, err
	}

	for _, s := range suggestions {
		_, err = tx.Exec(ctx,
			`INSERT INTO suggestions (record_id, title, description, priority, citation)
			 VALUES ($1, $2, $3, $4, $5)`,
			recordID, s.Title, s.Description, s.Priority, s.Citation,
		)
		if err != nil {
			return 0, err
		}
	}

	return recordID, tx.Commit(ctx)
}

// ─── GET /api/records ────────────────────────────────────────────────────────

func listRecords(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := db.Query(context.Background(),
			`SELECT id, filename, scribe_summary, created_at FROM patient_records ORDER BY created_at DESC LIMIT 50`)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var records []models.PatientRecord
		for rows.Next() {
			var r models.PatientRecord
			if err := rows.Scan(&r.ID, &r.Filename, &r.ScribeSummary, &r.CreatedAt); err != nil {
				continue
			}
			records = append(records, r)
		}
		c.JSON(http.StatusOK, records)
	}
}

// ─── GET /api/records/:id ────────────────────────────────────────────────────

func getRecord(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		ctx := context.Background()

		var r models.PatientRecord
		err := db.QueryRow(ctx,
			`SELECT id, filename, scribe_summary, created_at FROM patient_records WHERE id=$1`, id,
		).Scan(&r.ID, &r.Filename, &r.ScribeSummary, &r.CreatedAt)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "record not found"})
			return
		}

		rows, _ := db.Query(ctx,
			`SELECT id, title, description, priority, citation, created_at FROM suggestions WHERE record_id=$1 ORDER BY id`, id)
		defer rows.Close()

		var sugs []models.Suggestion
		for rows.Next() {
			var s models.Suggestion
			rows.Scan(&s.ID, &s.Title, &s.Description, &s.Priority, &s.Citation, &s.CreatedAt)
			sugs = append(sugs, s)
		}

		c.JSON(http.StatusOK, gin.H{"record": r, "suggestions": sugs})
	}
}

// ─── POST /api/emr/post/:id  (Mock EMR / MCP bonus) ─────────────────────────

func postToEMR(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		ctx := context.Background()

		var summary string
		err := db.QueryRow(ctx,
			`SELECT scribe_summary FROM patient_records WHERE id=$1`, id,
		).Scan(&summary)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "record not found"})
			return
		}

		// Mock EMR response — replace with real MCP tool call in production
		c.JSON(http.StatusOK, gin.H{
			"emr_status":   "posted",
			"encounter_id": fmt.Sprintf("ENC-%d-%d", id, time.Now().Unix()),
			"message":      "Treatment plan successfully posted to mock EMR (Epic FHIR sandbox).",
			"timestamp":    time.Now(),
		})
	}
}
