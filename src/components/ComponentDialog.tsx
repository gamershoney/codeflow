import { useEffect, useState } from 'react'
import { Box, Braces, Database, Globe2, Server, X } from 'lucide-react'
import type { Component, Operation, Project } from '../types'

type Props={open:boolean;project:Project;parentId:string;onClose:()=>void;onMutate:(ops:Operation[])=>Promise<void>}
const types=[
  {id:'service',label:'Service',icon:Server,description:'Runtime or responsibility'},
  {id:'database',label:'Database',icon:Database,description:'Durable data store'},
  {id:'client',label:'Client',icon:Globe2,description:'User-facing application'},
  {id:'api',label:'API',icon:Braces,description:'External contract'},
  {id:'generic',label:'Component',icon:Box,description:'General architecture unit'},
]

export default function ComponentDialog({open,project,parentId,onClose,onMutate}:Props){
  const [type,setType]=useState('service');const [title,setTitle]=useState('');const [description,setDescription]=useState('');const [parent,setParent]=useState(parentId);const [busy,setBusy]=useState(false)
  useEffect(()=>{if(open){setParent(parentId);setType('service');setTitle('');setDescription('')}},[open,parentId])
  if(!open)return null
  const submit=async(e:React.FormEvent)=>{e.preventDefault();if(!title.trim())return;setBusy(true);const id=`cmp_${crypto.randomUUID().slice(0,8)}`;const component:Component={id,type,title:title.trim(),description:description.trim()||'No description yet.',parentId:parent,provenance:{kind:'human',author:'You',confidence:1}};await onMutate([{kind:'add_component',component}]);setBusy(false);onClose()}
  return <div className="modal-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)onClose()}}><form className="component-dialog" onSubmit={submit}>
    <div className="dialog-head"><div><span className="eyebrow">Architecture model</span><h2>Add component</h2></div><button type="button" className="icon-button" onClick={onClose}><X size={16}/></button></div>
    <label className="field-label">Component type</label><div className="type-picker">{types.map(item=>{const Icon=item.icon;return <button type="button" key={item.id} className={type===item.id?'active':''} onClick={()=>setType(item.id)}><Icon size={16}/><span><b>{item.label}</b><small>{item.description}</small></span></button>})}</div>
    <label className="field-label" htmlFor="component-title">Name</label><input id="component-title" autoFocus placeholder="e.g. Notification service" value={title} onChange={e=>setTitle(e.target.value)}/>
    <label className="field-label" htmlFor="component-description">Responsibility</label><textarea id="component-description" placeholder="What does this component own?" value={description} onChange={e=>setDescription(e.target.value)}/>
    <label className="field-label" htmlFor="component-parent">Inside</label><select id="component-parent" value={parent} onChange={e=>setParent(e.target.value)}>{project.components.map(c=><option key={c.id} value={c.id}>{c.title}</option>)}</select>
    <div className="dialog-note">A stable ID will connect this component across the canvas, outline, blocks, API, and agent tools.</div>
    <div className="dialog-actions"><button type="button" onClick={onClose}>Cancel</button><button type="submit" disabled={!title.trim()||busy}>{busy?'Adding…':'Add component'}</button></div>
  </form></div>
}
