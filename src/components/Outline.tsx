import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, CircleDot, Database, FileCode2, Globe2, ListTree, Plus, Server } from 'lucide-react'
import type { Component, Project } from '../types'

type Props={project:Project; selected:string; onSelect:(id:string)=>void}
const iconMap:Record<string,typeof Server>={client:Globe2,service:Server,database:Database,system:ListTree}

export default function Outline({project,selected,onSelect}:Props){
  const [expanded,setExpanded]=useState<Set<string>>(new Set(['cmp_platform','cmp_backend']))
  const children=useMemo(()=>{
    const map:Record<string,Component[]>={}
    for(const component of project.components){ const p=component.parentId||'root'; (map[p]??=[]).push(component) }
    return map
  },[project.components])
  const toggle=(id:string)=>setExpanded(prev=>{const next=new Set(prev);next.has(id)?next.delete(id):next.add(id);return next})
  const row=(component:Component,depth:number):React.ReactNode=>{
    const nested=children[component.id]||[]; const open=expanded.has(component.id); const Icon=iconMap[component.type]||CircleDot
    return <div key={component.id}>
      <button className={`outline-row ${selected===component.id?'active':''}`} style={{paddingLeft:10+depth*16}} onClick={()=>onSelect(component.id)}>
        <span className="disclosure" onClick={e=>{e.stopPropagation();toggle(component.id)}}>{nested.length?(open?<ChevronDown/>:<ChevronRight/>):<span/>}</span>
        <Icon size={14}/><span>{component.title}</span><span className={`mini-kind ${component.type}`}/>
      </button>
      {open&&nested.map(child=>row(child,depth+1))}
      {open&&project.blocks.filter(b=>b.ownerComponentId===component.id&& !b.parentId).slice(0,3).map(block=><button key={block.id} className="outline-block" style={{paddingLeft:42+depth*16}} onClick={()=>onSelect(component.id)}><FileCode2 size={12}/><span>{block.title}</span></button>)}
    </div>
  }
  return <aside className="outline-panel">
    <div className="panel-label"><span>Architecture</span><button><Plus size={14}/></button></div>
    <div className="outline-tree">{(children.root||[]).map(item=>row(item,0))}</div>
    <div className="outline-footer"><div className="tiny-pulse"/><span>Live model</span><b>r{project.revision}</b></div>
  </aside>
}
