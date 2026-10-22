import { useEffect, useState } from 'react'
import { ArrowUpRight, Braces, Check, ChevronRight, Code2, Database, GitBranch, PencilLine, Plus, ShieldCheck, Trash2, X } from 'lucide-react'
import type { Block, Operation, Project } from '../types'

type Props={project:Project;selected:string;onMutate:(ops:Operation[])=>Promise<void>;onSelect:(id:string)=>void;onNotify:(message:string)=>void}
const blockIcons:Record<string,typeof Braces>={api:Braces,table:Database,code:Code2,requirement:ShieldCheck,behavior:GitBranch,constraint:ShieldCheck,note:PencilLine}
const blockTypes=['requirement','behavior','api','table','constraint','code','note']

export default function Inspector({project,selected,onMutate,onSelect,onNotify}:Props){
  const component=project.components.find(c=>c.id===selected) || project.components[0]
  const blocks=project.blocks.filter(b=>b.ownerComponentId===component?.id)
  const [title,setTitle]=useState(component?.title||'');const [description,setDescription]=useState(component?.description||'');const [editing,setEditing]=useState(false);const [addingBlock,setAddingBlock]=useState(false);const [editBlock,setEditBlock]=useState<Block|null>(null)
  useEffect(()=>{setTitle(component?.title||'');setDescription(component?.description||'');setEditing(false);setAddingBlock(false);setEditBlock(null)},[component?.id,component?.title,component?.description])
  if(!component)return null
  const save=async()=>{await onMutate([{kind:'update_component',targetId:component.id,title,description}]);setEditing(false)}
  const deleteComponent=async()=>{if(component.id==='cmp_platform')return;await onMutate([{kind:'delete_component',targetId:component.id}])}
  return <aside className="inspector">
    <div className="inspector-head"><span className={`large-kind ${component.type}`}>{component.type.slice(0,2).toUpperCase()}</span><div><span className="eyebrow">{component.type}</span>{editing?<input value={title} onChange={e=>setTitle(e.target.value)}/>:<h2>{component.title}</h2>}</div><button className="icon-button" onClick={()=>setEditing(v=>!v)}>{editing?<X size={15}/>:<PencilLine size={15}/>}</button></div>
    {editing?<><textarea value={description} onChange={e=>setDescription(e.target.value)}/><button className="save-button" onClick={save}><Check size={13}/> Save changes</button></>:<p className="component-description">{component.description}</p>}
    <div className="meta-row"><span>ID</span><code>{component.id}</code></div>
    <div className="meta-row"><span>Source</span><b><span className={`provenance ${component.provenance.kind}`}/>{component.provenance.kind}</b></div>
    {component.properties&&<div className="property-grid">{Object.entries(component.properties).map(([k,v])=><div key={k}><span>{k}</span><strong>{v}</strong></div>)}</div>}
    <div className="section-title"><span>Structured blocks</span><b>{blocks.length}</b><button className="section-add" onClick={()=>{setAddingBlock(true);setEditBlock(null)}}><Plus size={12}/></button></div>
    {(addingBlock||editBlock)&&<BlockEditor key={editBlock?.id||'new'} owner={component.id} block={editBlock} onCancel={()=>{setAddingBlock(false);setEditBlock(null)}} onSave={async block=>{await onMutate([{kind:editBlock?'update_block':'add_block',block}]);setAddingBlock(false);setEditBlock(null)}}/>}
    <div className="block-list">{blocks.length?blocks.map(block=><BlockCard block={block} key={block.id} onEdit={()=>{setEditBlock(block);setAddingBlock(false)}} onDelete={()=>onMutate([{kind:'delete_block',targetId:block.id}])}/>):<div className="empty-mini">No blocks yet. Add a requirement, contract, schema, or note.</div>}</div>
    <div className="section-title"><span>Relationships</span><b>{project.relationships.filter(r=>r.sourceId===component.id||r.targetId===component.id).length}</b></div>
    <div className="relationship-list">{project.relationships.filter(r=>r.sourceId===component.id||r.targetId===component.id).map(r=>{const outgoing=r.sourceId===component.id;const peer=project.components.find(c=>c.id===(outgoing?r.targetId:r.sourceId));return <div className="relationship-row" key={r.id}><button onClick={()=>peer&&onSelect(peer.id)}><GitBranch size={13}/><span>{outgoing?'to':'from'} <b>{peer?.title}</b><small>{r.label}</small></span><ChevronRight size={14}/></button><button className="row-delete" title="Remove relationship" onClick={()=>onMutate([{kind:'disconnect',targetId:r.id}])}><X size={12}/></button></div>})}</div>
    <button className="source-link" onClick={async()=>{await navigator.clipboard.writeText(`codeflow://project/${project.id}/component/${component.id}`);onNotify('Portable source reference copied')}}><Code2 size={14}/> Copy portable source reference <ArrowUpRight size={13}/></button>
    {component.id!=='cmp_platform'&&<button className="danger-link" onClick={deleteComponent}><Trash2 size={13}/> Delete component</button>}
  </aside>
}

function BlockEditor({owner,block,onCancel,onSave}:{owner:string;block:Block|null;onCancel:()=>void;onSave:(block:Block)=>Promise<void>}){
  const [type,setType]=useState(block?.type||'requirement');const [title,setTitle]=useState(block?.title||'');const [content,setContent]=useState(block?.content||'');const [properties,setProperties]=useState(block?.properties||{});const [busy,setBusy]=useState(false)
  const submit=async(e:React.FormEvent)=>{e.preventDefault();if(!title.trim())return;setBusy(true);await onSave({id:block?.id||`blk_${crypto.randomUUID().slice(0,8)}`,ownerComponentId:owner,parentId:block?.parentId,type,title:title.trim(),content:content.trim(),properties,childIds:block?.childIds});setBusy(false)}
  return <form className="block-editor" onSubmit={submit}><div className="block-editor-top"><select value={type} onChange={e=>setType(e.target.value)}>{blockTypes.map(t=><option key={t}>{t}</option>)}</select><button type="button" onClick={onCancel}><X size={12}/></button></div><input autoFocus placeholder="Block title" value={title} onChange={e=>setTitle(e.target.value)}/><textarea placeholder="Describe the behavior, contract, schema, or decision…" value={content} onChange={e=>setContent(e.target.value)}/>{(type==='api'||type==='table')&&<input placeholder={type==='api'?'Path, e.g. /v1/orders':'Columns, e.g. id uuid PK'} value={type==='api'?(properties.path||''):(properties.columns||'')} onChange={e=>setProperties({...properties,[type==='api'?'path':'columns']:e.target.value})}/>}<button type="submit" disabled={!title.trim()||busy}>{busy?'Saving…':block?'Update block':'Add block'}</button></form>
}

function BlockCard({block,onEdit,onDelete}:{block:Block;onEdit:()=>void;onDelete:()=>void}){const Icon=blockIcons[block.type]||Braces;return <div className={`block-card block-${block.type}`}><div className="block-card-head"><Icon size={13}/><span>{block.type}</span><code>{block.id.replace('blk_','')}</code><button onClick={onEdit}><PencilLine size={10}/></button><button onClick={onDelete}><Trash2 size={10}/></button></div><h4>{block.title}</h4><p>{block.content}</p>{block.properties&&<div className="block-properties">{Object.entries(block.properties).map(([k,v])=><div key={k}><b>{k}</b><pre>{v}</pre></div>)}</div>}</div>}
