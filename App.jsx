import { useState, useRef, useCallback } from 'react'
import './App.css'

const API = '/api'

const STAGES = [
  { label: 'Upload', icon: 'ti-upload' },
  { label: 'Agent A: Scribe', icon: 'ti-file-text' },
  { label: 'Agent B: Specialist', icon: 'ti-stethoscope' },
  { label: 'Saving', icon: 'ti-database' },
  { label: 'Complete', icon: 'ti-circle-check' },
]

const PRIORITY_CONFIG = {
  urgent: { label: 'Urgent', cls: 'badge-danger' },
  high:   { label: 'High priority', cls: 'badge-warn' },
  moderate: { label: 'Moderate', cls: 'badge-info' },
}

export default function App() {
  const [tab, setTab] = useState('upload')
  const [drag, setDrag] = useState(false)
  const [stage, setStage] = useState(0)
  const [stageLabel, setStageLabel] = useState('')
  const [scribeSummary, setScribeSummary] = useState('')
  const [suggestions, setSuggestions] = useState([])
  const [recordId, setRecordId] = useState(null)
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(false)
  const [emrStatus, setEmrStatus] = useState(null)
  const [history, setHistory] = useState([])
  const fileRef = useRef()

  // ── SSE upload pipeline ─────────────────────────────────────────────────
  const handleFile = useCallback(async (file) => {
    if (!file || file.type !== 'application/pdf') {
      setError('Please upload a PDF file.')
      return
    }
    setError(null)
    setStage(0)
    setScribeSummary('')
    setSuggestions([])
    setRecordId(null)
    setEmrStatus(null)
    setLoading(true)
    setTab('results')

    const form = new FormData()
    form.append('file', file)

    try {
      const resp = await fetch(`${API}/upload`, { method: 'POST', body: form })
      if (!resp.ok) throw new Error(`Server error ${resp.status}`)

      const reader = resp.body.getReader()
      const decoder = new TextDecoder()
      let buf = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })

        // Parse SSE lines
        const parts = buf.split('\n\n')
        buf = parts.pop() // keep incomplete chunk

        for (const part of parts) {
          const lines = part.split('\n')
          let eventType = 'message'
          let dataStr = ''
          for (const line of lines) {
            if (line.startsWith('event: ')) eventType = line.slice(7).trim()
            if (line.startsWith('data: ')) dataStr = line.slice(6).trim()
          }
          if (!dataStr) continue
          try {
            const payload = JSON.parse(dataStr)
            handleSSEEvent(eventType, payload)
          } catch {}
        }
      }
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  const handleSSEEvent = (type, payload) => {
    switch (type) {
      case 'pipeline_stage':
        setStage(payload.stage)
        setStageLabel(payload.label || '')
        break
      case 'scribe_done':
        setScribeSummary(payload.summary)
        break
      case 'suggestions':
        setSuggestions(payload)
        setTab('suggestions')
        break
      case 'record_id':
        setRecordId(payload.id)
        break
      case 'done':
        setStage(4)
        setLoading(false)
        fetchHistory()
        break
      case 'error':
        setError(payload.message)
        setLoading(false)
        break
    }
  }

  const fetchHistory = async () => {
    try {
      const r = await fetch(`${API}/records`)
      const data = await r.json()
      setHistory(data || [])
    } catch {}
  }

  const postToEMR = async () => {
    if (!recordId) return
    try {
      const r = await fetch(`${API}/emr/post/${recordId}`, { method: 'POST' })
      const data = await r.json()
      setEmrStatus(data)
    } catch (e) {
      setEmrStatus({ emr_status: 'error', message: e.message })
    }
  }

  // ── Drag & drop ──────────────────────────────────────────────────────────
  const onDrop = (e) => {
    e.preventDefault()
    setDrag(false)
    handleFile(e.dataTransfer.files[0])
  }

  return (
    <div className="layout">
      {/* ── Sidebar ── */}
      <aside className="sidebar">
        <div className="sidebar-logo">
          <div className="brand">
            <div className="brand-icon"><i className="ti ti-heart-rate-monitor" /></div>
            <div>
              <div className="brand-name">Sovereign</div>
              <div className="brand-sub">Patient Copilot</div>
            </div>
          </div>
        </div>

        <nav className="sidebar-nav">
          <div className="nav-section">
            <div className="nav-label">Workflow</div>
            <NavItem icon="ti-upload" label="Upload Record" active={tab === 'upload'} onClick={() => setTab('upload')} />
            <NavItem icon="ti-report-medical" label="Patient Summary" active={tab === 'results'} onClick={() => stage > 0 && setTab('results')} dim={stage === 0} />
            <NavItem icon="ti-bulb" label="AI Suggestions" active={tab === 'suggestions'} onClick={() => suggestions.length && setTab('suggestions')} dim={!suggestions.length} />
          </div>
          <div className="nav-section">
            <div className="nav-label">Agents</div>
            <NavItem icon="ti-file-text" label="Agent A — Scribe" active={stage === 1 && loading} />
            <NavItem icon="ti-stethoscope" label="Agent B — Specialist" active={stage === 2 && loading} />
          </div>
          <div className="nav-section">
            <div className="nav-label">Records</div>
            <NavItem icon="ti-database" label="History" onClick={() => { fetchHistory(); setTab('history') }} active={tab === 'history'} />
            <NavItem icon="ti-send" label="Post to EMR" onClick={postToEMR} dim={!recordId} />
          </div>
        </nav>

        {stage > 0 && (
          <div className="sidebar-status">
            <div className="status-dot" data-done={stage === 4} data-active={loading} />
            <span>{stage === 4 ? 'Analysis complete' : stageLabel || 'Processing…'}</span>
          </div>
        )}
      </aside>

      {/* ── Main ── */}
      <main className="main">
        {/* Topbar */}
        <div className="topbar">
          <div>
            <div className="topbar-title">
              {tab === 'upload' && 'Upload Patient Record'}
              {tab === 'results' && 'Patient Summary'}
              {tab === 'suggestions' && 'AI Clinical Suggestions'}
              {tab === 'history' && 'Record History'}
            </div>
            <div className="topbar-meta">Two-agent AI pipeline · Data stays local · Go + Qdrant + Postgres</div>
          </div>
          <div className="topbar-badges">
            {stage === 4 && <span className="badge badge-success"><i className="ti ti-circle-check" /> Complete</span>}
            {loading && <span className="badge badge-info"><i className="ti ti-loader" style={{animation:'spin 1s linear infinite',display:'inline-block'}} /> Processing…</span>}
          </div>
        </div>

        {/* Pipeline strip */}
        {stage > 0 && (
          <div className="pipeline">
            {STAGES.map((s, i) => (
              <div key={i} className={`pipeline-step ${i < stage ? 'done' : i === stage ? 'active' : 'pending'}`}>
                <div className="step-icon">
                  {i < stage
                    ? <i className="ti ti-check" />
                    : (i === stage && loading)
                    ? <i className="ti ti-loader" style={{animation:'spin 1s linear infinite',display:'inline-block'}} />
                    : <i className={`ti ${s.icon}`} />
                  }
                </div>
                <span className="step-label">{s.label}</span>
              </div>
            ))}
          </div>
        )}

        {error && (
          <div className="error-banner">
            <i className="ti ti-alert-circle" /> {error}
            <button onClick={() => setError(null)}><i className="ti ti-x" /></button>
          </div>
        )}

        <div className="content">

          {/* ── Upload tab ── */}
          {tab === 'upload' && (
            <div style={{display:'flex',flexDirection:'column',gap:16}}>
              <div
                className={`upload-zone ${drag ? 'drag' : ''}`}
                onDragOver={e => { e.preventDefault(); setDrag(true) }}
                onDragLeave={() => setDrag(false)}
                onDrop={onDrop}
                onClick={() => fileRef.current.click()}
              >
                <i className={`ti ${drag ? 'ti-file-upload' : 'ti-file-type-pdf'}`} style={{fontSize:40,color:'var(--gray-600)',marginBottom:10,display:'block'}} />
                <div className="upload-title">Drop a patient PDF here</div>
                <div className="upload-sub">PDF only · Processed locally · No data leaves your machine</div>
                <button className="upload-btn" onClick={e => { e.stopPropagation(); fileRef.current.click() }}>
                  <i className="ti ti-upload" /> Browse file
                </button>
                <input ref={fileRef} type="file" accept=".pdf" style={{display:'none'}} onChange={e => handleFile(e.target.files[0])} />
              </div>

              <Card title="How the agents work" icon="ti-info-circle">
                <div className="agent-explainer">
                  <AgentRow chip="A" chipClass="chip-scribe" name="Scribe" model="claude-haiku-4-5" desc="Fast extraction & de-identification. Strips PHI, pulls vitals, labs, diagnoses. Routed to Haiku for speed and cost efficiency." />
                  <AgentRow chip="B" chipClass="chip-specialist" name="Specialist" model="claude-sonnet-4-6" desc="Deep clinical reasoning. Receives the clean summary and returns 4 evidence-based next-best actions with PDF citations. Routed to Sonnet for richer reasoning." />
                </div>
                <div className="privacy-note">
                  <i className="ti ti-lock" /> All PHI is stripped by Agent A before any data reaches Agent B or is stored in Postgres.
                </div>
              </Card>
            </div>
          )}

          {/* ── Results tab ── */}
          {tab === 'results' && (
            <div style={{display:'flex',flexDirection:'column',gap:14}}>
              {stage === 0 && <EmptyState icon="ti-file-off" text="Upload a record to see the patient summary." />}

              {stage >= 1 && (
                <>
                  <Card title="Scribe Summary" icon="ti-file-text" badge={<AgentChip label="Agent A" cls="chip-scribe" />}>
                    {scribeSummary
                      ? <div className="summary-box" style={{animation:'fadeIn 0.3s'}}>{scribeSummary}</div>
                      : <div className="summary-box streaming"><span className="cursor" /></div>
                    }
                  </Card>

                  {suggestions.length > 0 && (
                    <div style={{textAlign:'center'}}>
                      <button className="btn btn-primary" onClick={() => setTab('suggestions')}>
                        <i className="ti ti-bulb" /> View AI Suggestions →
                      </button>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {/* ── Suggestions tab ── */}
          {tab === 'suggestions' && (
            <div style={{display:'flex',flexDirection:'column',gap:14}}>
              {!suggestions.length && stage < 3 && (
                <EmptyState icon="ti-loader" text="Specialist agent is generating suggestions…" spin />
              )}
              {!suggestions.length && stage === 0 && (
                <EmptyState icon="ti-file-off" text="Upload a patient record to see AI suggestions." />
              )}

              {suggestions.length > 0 && (
                <>
                  <div className="row-between">
                    <div style={{display:'flex',gap:8,alignItems:'center'}}>
                      <AgentChip label="Agent B — Specialist" cls="chip-specialist" />
                      <span style={{fontSize:12,color:'var(--gray-600)'}}>{suggestions.length} next best actions</span>
                    </div>
                    <button className="btn btn-outline" onClick={postToEMR} disabled={!recordId}>
                      <i className="ti ti-send" /> Post to EMR
                    </button>
                  </div>

                  {emrStatus && (
                    <div className={`emr-banner ${emrStatus.emr_status === 'error' ? 'emr-error' : 'emr-ok'}`}>
                      <i className={`ti ${emrStatus.emr_status === 'error' ? 'ti-alert-circle' : 'ti-circle-check'}`} />
                      {emrStatus.message}
                      {emrStatus.encounter_id && <span className="encounter-id">{emrStatus.encounter_id}</span>}
                    </div>
                  )}

                  <Card title="Next Best Actions" icon="ti-list-check">
                    {suggestions.map((s, i) => (
                      <div key={i} className="suggestion-item" style={{animation:`fadeIn 0.3s ${i * 0.07}s both`}}>
                        <div className="sug-row">
                          <div className="sug-num">{i + 1}</div>
                          <div style={{flex:1}}>
                            <div className="sug-title">{s.title}</div>
                            <span className={`badge ${(PRIORITY_CONFIG[s.priority] || PRIORITY_CONFIG.moderate).cls}`}>
                              {(PRIORITY_CONFIG[s.priority] || PRIORITY_CONFIG.moderate).label}
                            </span>
                          </div>
                        </div>
                        <div className="sug-desc">{s.description}</div>
                        {s.citation && (
                          <div className="citation">
                            <i className="ti ti-quote" /> {s.citation}
                          </div>
                        )}
                      </div>
                    ))}
                  </Card>

                  <div className="privacy-note">
                    <i className="ti ti-shield-check" /> AI suggestions must be reviewed by a qualified clinician before acting.
                  </div>
                </>
              )}
            </div>
          )}

          {/* ── History tab ── */}
          {tab === 'history' && (
            <Card title="Uploaded Records" icon="ti-history">
              {!history?.length && <EmptyState icon="ti-inbox" text="No records yet." />}
              {history?.map(r => (
                <div key={r.id} className="history-row" onClick={() => {
                  setScribeSummary(r.scribe_summary)
                  setTab('results')
                  setStage(4)
                }}>
                  <div style={{display:'flex',alignItems:'center',gap:10}}>
                    <i className="ti ti-file-type-pdf" style={{color:'var(--red-600)',fontSize:18}} />
                    <div>
                      <div style={{fontWeight:500,fontSize:13}}>{r.filename}</div>
                      <div style={{fontSize:11,color:'var(--gray-600)'}}>{new Date(r.created_at).toLocaleString()}</div>
                    </div>
                  </div>
                  <i className="ti ti-arrow-right" style={{color:'var(--gray-600)'}} />
                </div>
              ))}
            </Card>
          )}

        </div>
      </main>
    </div>
  )
}

// ── Small components ─────────────────────────────────────────────────────────

function NavItem({ icon, label, active, onClick, dim }) {
  return (
    <div className={`nav-item ${active ? 'active' : ''} ${dim ? 'dim' : ''}`} onClick={onClick}>
      <i className={`ti ${icon}`} />
      <span>{label}</span>
    </div>
  )
}

function Card({ title, icon, badge, children }) {
  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title"><i className={`ti ${icon}`} />{title}</span>
        {badge}
      </div>
      <div className="card-body">{children}</div>
    </div>
  )
}

function AgentChip({ label, cls }) {
  return <span className={`agent-chip ${cls}`}>{label}</span>
}

function AgentRow({ chip, chipClass, name, model, desc }) {
  return (
    <div className="agent-row">
      <span className={`agent-chip ${chipClass}`}>Agent {chip}</span>
      <div>
        <div style={{fontWeight:500,fontSize:13,marginBottom:2}}>{name} <span style={{fontSize:11,color:'var(--gray-600)',fontWeight:400}}>→ {model}</span></div>
        <div style={{fontSize:12,color:'var(--gray-600)',lineHeight:1.5}}>{desc}</div>
      </div>
    </div>
  )
}

function EmptyState({ icon, text, spin }) {
  return (
    <div className="empty-state">
      <i className={`ti ${icon}`} style={spin ? {animation:'spin 1s linear infinite',display:'inline-block'} : {}} />
      <div>{text}</div>
    </div>
  )
}
