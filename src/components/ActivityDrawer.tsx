import { GitCommitHorizontal, History, Sparkles, X } from 'lucide-react'
import type { Project } from '../types'

type Props={project:Project;open:boolean;onClose:()=>void}
export default function ActivityDrawer({project,open,onClose}:Props){if(!open)return null;return <div className="drawer-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)onClose()}}><aside className="activity-drawer">
  <div className="drawer-top"><div><span className="proposal-kicker"><History size={13}/> Revision history</span><h2>Architecture activity</h2></div><button className="icon-button" onClick={onClose}><X size={17}/></button></div>
  <div className="revision-card"><span>Current revision</span><b>r{project.revision}</b><small>{project.components.length} components · {project.relationships.length} relationships · {project.blocks.length} blocks</small></div>
  <div className="timeline">{project.history.map((event,index)=><div className="timeline-event" key={event.id}><span className="timeline-icon">{event.kind.includes('proposal')?<Sparkles size={13}/>:<GitCommitHorizontal size={13}/>}</span><div><b>{event.summary}</b><span>{event.author} · revision {event.revision}</span><small>{new Date(event.createdAt).toLocaleString()}</small></div>{index<project.history.length-1&&<i/>}</div>)}</div>
</aside></div>}
