import { useEffect, useMemo, useState } from 'react'
import { Bell, Blocks, Box, Check, Copy, FolderSearch, GitBranch, History, Link2, LoaderCircle, Search, Sparkles, X } from 'lucide-react'
import { api } from '../api'
import type { Component, DiscoveryReport, Project } from '../types'

type SearchAction={id:string;label:string;description:string;icon:typeof Box;run:()=>void}

export function SearchPalette({project,open,onClose,onSelect,onAdd,onReview,onActivity,onDiscover}:{project:Project;open:boolean;onClose:()=>void;onSelect:(id:string)=>void;onAdd:()=>void;onReview:()=>void;onActivity:()=>void;onDiscover:()=>void}){
  const [query,setQuery]=useState('')
  useEffect(()=>{if(open)setQuery('')},[open])
  const actions=useMemo<SearchAction[]>(()=>[
    ...project.components.map(component=>({id:`component-${component.id}`,label:component.title,description:`${component.type} · ${component.description}`,icon:Box,run:()=>onSelect(component.id)})),
    ...project.blocks.map(block=>({id:`block-${block.id}`,label:block.title,description:`${block.type} block · ${project.components.find(c=>c.id===block.ownerComponentId)?.title||'Unknown component'}`,icon:Blocks,run:()=>onSelect(block.ownerComponentId)})),
    ...project.relationships.map(relationship=>({id:`relationship-${relationship.id}`,label:relationship.label,description:`${project.components.find(c=>c.id===relationship.sourceId)?.title||relationship.sourceId} → ${project.components.find(c=>c.id===relationship.targetId)?.title||relationship.targetId}`,icon:GitBranch,run:()=>onSelect(relationship.sourceId)})),
    {id:'action-add',label:'Add component',description:'Create a new architecture component',icon:Box,run:onAdd},
    {id:'action-review',label:'Review proposals',description:'Inspect pending agent changes',icon:Sparkles,run:onReview},
    {id:'action-activity',label:'Open activity',description:'View revision history',icon:History,run:onActivity},
    {id:'action-discover',label:'Discover repository',description:'Inspect a local codebase without modifying it',icon:FolderSearch,run:onDiscover},
  ],[project,onSelect,onAdd,onReview,onActivity,onDiscover])
  const results=actions.filter(item=>`${item.label} ${item.description}`.toLowerCase().includes(query.trim().toLowerCase())).slice(0,12)
  if(!open)return null
  const choose=(item:SearchAction)=>{item.run();onClose()}
  return <div className="command-backdrop" onMouseDown={event=>{if(event.target===event.currentTarget)onClose()}}>
    <div className="command-palette" role="dialog" aria-modal="true" aria-label="Search architecture">
      <div className="command-input"><Search size={17}/><input autoFocus value={query} onChange={event=>setQuery(event.target.value)} onKeyDown={event=>{if(event.key==='Escape')onClose();if(event.key==='Enter'&&results[0])choose(results[0])}} placeholder="Search components, blocks, relationships, or actions…"/><kbd>Esc</kbd></div>
      <div className="command-results">{results.length?results.map((item,index)=>{const Icon=item.icon;return <button key={item.id} onClick={()=>choose(item)}><span className="command-icon"><Icon size={14}/></span><span><b>{item.label}</b><small>{item.description}</small></span>{index===0&&<kbd>↵</kbd>}</button>}):<div className="command-empty">No architecture items match “{query}”.</div>}</div>
      <div className="command-footer"><span>Searches the canonical graph</span><span><kbd>↑</kbd><kbd>↓</kbd> navigate · <kbd>↵</kbd> open</span></div>
    </div>
  </div>
}

export function RepositoryDiscoveryDialog({open,onClose}:{open:boolean;onClose:()=>void}){
  const [path,setPath]=useState('')
  const [report,setReport]=useState<DiscoveryReport|null>(null)
  const [error,setError]=useState('')
  const [busy,setBusy]=useState(false)
  if(!open)return null
  const scan=async(event:React.FormEvent)=>{event.preventDefault();if(!path.trim())return;setBusy(true);setError('');setReport(null);try{setReport(await api.discover(path.trim()))}catch(reason){setError((reason as Error).message)}finally{setBusy(false)}}
  const componentById=(id:string)=>report?.components.find(item=>item.id===id)
  return <div className="modal-backdrop" onMouseDown={event=>{if(event.target===event.currentTarget)onClose()}}><div className="discovery-dialog">
    <div className="dialog-head"><div><span className="eyebrow">Repository intelligence</span><h2>Discover a local codebase</h2></div><button className="icon-button" onClick={onClose}><X size={16}/></button></div>
    <p className="discovery-note">Read-only scan. Paths must be inside <code>CODEFLOW_DISCOVERY_ROOTS</code>; source and architecture data are not changed.</p>
    <form className="discovery-form" onSubmit={scan}><input autoFocus value={path} onChange={event=>setPath(event.target.value)} placeholder="H:\ProjectsFolder\YourProject"/><button disabled={!path.trim()||busy}>{busy?<LoaderCircle className="spin" size={14}/>:<FolderSearch size={14}/>} {busy?'Scanning…':'Scan repository'}</button></form>
    {error&&<div className="discovery-error">{error}</div>}
    {report&&<div className="discovery-results">
      <div className="discovery-summary"><div><span>{report.kind}</span><h3>{report.name}</h3><small>{report.root}</small></div><div><b>{report.sourceFiles}</b><span>source files</span></div><div><b>{report.components.length}</b><span>components</span></div><div><b>{report.dependencies.length}</b><span>dependencies</span></div></div>
      <div className="discovery-tags">{report.frameworks.map(item=><span key={item}>{item}</span>)}{Object.entries(report.languages).sort((a,b)=>b[1]-a[1]).slice(0,6).map(([name,count])=><span key={name}>{name} · {count}</span>)}</div>
      {report.warnings?.map(warning=><div className="discovery-warning" key={warning}>{warning}</div>)}
      <div className="discovery-columns"><section><h4>Detected components</h4>{report.components.slice(0,40).map(item=><div className="discovery-row" key={item.id}><span><b>{item.name}</b><small>{item.path}</small></span><em>{item.language} · {item.fileCount}</em></div>)}</section><section><h4>Evidence-backed dependencies</h4>{report.dependencies.slice(0,40).map(item=><div className="discovery-row" key={`${item.sourceId}-${item.targetId}`}><span><b>{componentById(item.sourceId)?.name} → {componentById(item.targetId)?.name}</b><small>{item.evidence[0]}</small></span><em>{item.count} imports</em></div>)}</section></div>
    </div>}
  </div></div>
}

export function ShareDialog({project,open,onClose,onNotify}:{project:Project;open:boolean;onClose:()=>void;onNotify:(message:string)=>void}){
  if(!open)return null
  const copy=async()=>{await navigator.clipboard.writeText(window.location.href);onNotify('Workspace link copied')}
  return <div className="modal-backdrop" onMouseDown={event=>{if(event.target===event.currentTarget)onClose()}}><div className="small-dialog">
    <div className="dialog-head"><div><span className="eyebrow">Collaboration</span><h2>Share {project.name}</h2></div><button className="icon-button" onClick={onClose}><X size={16}/></button></div>
    <p>Open this link in another browser on this host. Remote sharing requires exposing the self-hosted server; architecture changes remain revisioned and attributed.</p>
    <div className="share-link"><Link2 size={14}/><span>{window.location.href}</span><button onClick={copy}><Copy size={13}/> Copy</button></div>
    <div className="share-access"><span className="user-avatar">BS</span><div><b>You</b><small>Workspace owner · can edit and review</small></div><Check size={15}/></div>
  </div></div>
}

export type CanvasPreferences={showGrid:boolean;showMinimap:boolean;snapToGrid:boolean}
export function SettingsDialog({open,value,onChange,onClose}:{open:boolean;value:CanvasPreferences;onChange:(value:CanvasPreferences)=>void;onClose:()=>void}){
  if(!open)return null
  const toggle=(key:keyof CanvasPreferences)=>onChange({...value,[key]:!value[key]})
  return <div className="modal-backdrop" onMouseDown={event=>{if(event.target===event.currentTarget)onClose()}}><div className="small-dialog settings-dialog">
    <div className="dialog-head"><div><span className="eyebrow">Canvas</span><h2>View settings</h2></div><button className="icon-button" onClick={onClose}><X size={16}/></button></div>
    {([['showGrid','Dot grid','Show the visual alignment grid'],['showMinimap','Minimap','Show the project overview'],['snapToGrid','Snap to grid','Align moved nodes to 20px increments']] as [keyof CanvasPreferences,string,string][]).map(([key,label,description])=><button className="setting-row" key={key} onClick={()=>toggle(key)}><span><b>{label}</b><small>{description}</small></span><i className={value[key]?'on':''}><span/></i></button>)}
  </div></div>
}

export function ProjectMenu({project,open,onClose}:{project:Project;open:boolean;onClose:()=>void}){
  if(!open)return null
  return <div className="popover project-menu">
    <span className="popover-label">Current project</span>
    <button className="project-menu-item" onClick={onClose}><span className="project-glyph"><Box size={14}/></span><span><b>{project.name}</b><small>{project.summary}</small></span><Check size={14}/></button>
    <div className="project-stats"><span><b>{project.components.length}</b> components</span><span><b>{project.blocks.length}</b> blocks</span><span><b>r{project.revision}</b> revision</span></div>
  </div>
}

export function NotificationsPopover({project,open,onReview,onActivity}:{project:Project;open:boolean;onReview:()=>void;onActivity:()=>void}){
  if(!open)return null
  const pending=project.proposals.filter(item=>item.status==='pending')
  return <div className="popover notifications-popover">
    <div className="popover-head"><span><Bell size={13}/> Notifications</span><b>{pending.length}</b></div>
    {pending.map(proposal=><button key={proposal.id} onClick={onReview}><span className="notification-icon"><Sparkles size={13}/></span><span><b>{proposal.title}</b><small>{proposal.author} proposed {proposal.operations.length} changes</small></span></button>)}
    {!pending.length&&<div className="notification-empty"><Check size={15}/><span>You’re all caught up</span></div>}
    <button className="notification-footer" onClick={onActivity}><History size={12}/> View all activity</button>
  </div>
}

export function CatalogView({kind,project,onSelect}:{kind:'relationships'|'blocks';project:Project;onSelect:(component:Component)=>void}){
  const component=(id:string)=>project.components.find(item=>item.id===id)
  return <main className="catalog-area">
    <header><div><span className="eyebrow">Project index</span><h1>{kind==='relationships'?'Relationships':'Structured blocks'}</h1><p>{kind==='relationships'?'Explore every typed dependency in the current architecture.':'Browse requirements, contracts, schemas, decisions, and notes.'}</p></div><b>{kind==='relationships'?project.relationships.length:project.blocks.length}</b></header>
    <div className="catalog-list">{kind==='relationships'?project.relationships.map(relationship=>{const source=component(relationship.sourceId);const target=component(relationship.targetId);return <button key={relationship.id} onClick={()=>source&&onSelect(source)}><span className="catalog-icon"><GitBranch size={15}/></span><span><b>{source?.title||relationship.sourceId} <em>→</em> {target?.title||relationship.targetId}</b><small>{relationship.kind} · {relationship.label}</small></span><span className={`catalog-provenance ${relationship.provenance.kind}`}>{relationship.provenance.kind}</span></button>}):project.blocks.map(block=>{const owner=component(block.ownerComponentId);return <button key={block.id} onClick={()=>owner&&onSelect(owner)}><span className="catalog-icon"><Blocks size={15}/></span><span><b>{block.title}</b><small>{block.type} · {owner?.title||block.ownerComponentId}</small></span><span className="catalog-preview">{block.content}</span></button>})}</div>
  </main>
}
