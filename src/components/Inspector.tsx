import { useEffect, useState } from 'react'
import { ArrowUpRight, Braces, Check, ChevronRight, Code2, Database, GitBranch, PencilLine, ShieldCheck } from 'lucide-react'
import type { Block, Component, Operation, Project } from '../types'

type Props={project:Project; selected:string; onMutate:(ops:Operation[])=>Promise<void>}
const blockIcons:Record<string,typeof Braces>={api:Braces,table:Database,code:Code2,requirement:ShieldCheck,behavior:GitBranch,constraint:ShieldCheck}

export default function Inspector({project,selected,onMutate}:Props){
  const component=project.components.find(c=>c.id===selected) || project.components[0]
  const blocks=project.blocks.filter(b=>b.ownerComponentId===component?.id)
  const [title,setTitle]=useState(component?.title||''); const [description,setDescription]=useState(component?.description||''); const [editing,setEditing]=useState(false)
  useEffect(()=>{setTitle(component?.title||'');setDescription(component?.description||'');setEditing(false)},[component?.id,component?.title,component?.description])
  if(!component)return null
  const save=async()=>{await onMutate([{kind:'update_component',targetId:component.id,title,description}]);setEditing(false)}
  return <aside className="inspector">
    <div className="inspector-head"><span className={`large-kind ${component.type}`}>{component.type.slice(0,2).toUpperCase()}</span><div><span className="eyebrow">{component.type}</span>{editing?<input value={title} onChange={e=>setTitle(e.target.value)}/>:<h2>{component.title}</h2>}</div><button className="icon-button" onClick={()=>setEditing(v=>!v)}>{editing?<Check size={15}/>:<PencilLine size={15}/>}</button></div>
    {editing?<><textarea value={description} onChange={e=>setDescription(e.target.value)}/><button className="save-button" onClick={save}>Save changes</button></>:<p className="component-description">{component.description}</p>}
    <div className="meta-row"><span>ID</span><code>{component.id}</code></div>
    <div className="meta-row"><span>Source</span><b><span className={`provenance ${component.provenance.kind}`}/>{component.provenance.kind}</b></div>
    {component.properties&&<div className="property-grid">{Object.entries(component.properties).map(([k,v])=><div key={k}><span>{k}</span><strong>{v}</strong></div>)}</div>}
    <div className="section-title"><span>Structured blocks</span><b>{blocks.length}</b></div>
    <div className="block-list">{blocks.length?blocks.map(block=><BlockCard block={block} key={block.id}/>):<div className="empty-mini">No blocks in this component yet.</div>}</div>
    <div className="section-title"><span>Relationships</span></div>
    <div className="relationship-list">{project.relationships.filter(r=>r.sourceId===component.id||r.targetId===component.id).map(r=>{const outgoing=r.sourceId===component.id;const peer=project.components.find(c=>c.id===(outgoing?r.targetId:r.sourceId));return <button key={r.id}><GitBranch size={13}/><span>{outgoing?'to':'from'} <b>{peer?.title}</b><small>{r.label}</small></span><ChevronRight size={14}/></button>})}</div>
    <button className="source-link"><Code2 size={14}/> Copy portable source reference <ArrowUpRight size={13}/></button>
  </aside>
}

function BlockCard({block}:{block:Block}){const Icon=blockIcons[block.type]||Braces;return <div className={`block-card block-${block.type}`}><div className="block-card-head"><Icon size={13}/><span>{block.type}</span><code>{block.id.replace('blk_','')}</code></div><h4>{block.title}</h4><p>{block.content}</p>{block.properties&&<div className="block-properties">{Object.entries(block.properties).map(([k,v])=><div key={k}><b>{k}</b><pre>{v}</pre></div>)}</div>}</div>}
