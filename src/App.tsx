import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Background, BackgroundVariant, Controls, MarkerType, MiniMap, ReactFlow, useNodesState, type Connection, type Edge, type Node, type NodeMouseHandler, type NodeTypes, type OnNodeDrag } from '@xyflow/react'
import { Activity, Bell, Box, ChevronDown, ChevronRight, Command, GitBranch, Grid2X2, History, LayoutDashboard, Menu, MessageSquare, MousePointer2, Plus, Search, Settings2, Sparkles, Users, Waypoints, ZoomIn } from 'lucide-react'
import { api } from './api'
import type { Operation, Presence, Project } from './types'
import { ArchitectureNode } from './components/ArchitectureNode'
import Outline from './components/Outline'
import Inspector from './components/Inspector'
import ProposalDrawer from './components/ProposalDrawer'
import ComponentDialog from './components/ComponentDialog'
import ActivityDrawer from './components/ActivityDrawer'

const nodeTypes:NodeTypes={architecture:ArchitectureNode}
const sessionId=sessionStorage.getItem('codeflow-session')||crypto.randomUUID()
sessionStorage.setItem('codeflow-session',sessionId)

export default function App(){
  const [project,setProject]=useState<Project|null>(null)
  const [selected,setSelected]=useState('cmp_backend')
  const [scope,setScope]=useState('cmp_platform')
  const [proposalOpen,setProposalOpen]=useState(false)
  const [componentDialogOpen,setComponentDialogOpen]=useState(false)
  const [activityOpen,setActivityOpen]=useState(false)
  const [presence,setPresence]=useState<Presence[]>([])
  const [toast,setToast]=useState('')
  const [loading,setLoading]=useState(true)
  const [flowNodes,setFlowNodes,onNodesChange]=useNodesState<Node>([])
  const draggingRef=useRef(false)
  const projectRef=useRef(project); projectRef.current=project
  const refresh=useCallback(async()=>{try{setProject(await api.project())}catch(e){setToast((e as Error).message)}finally{setLoading(false)}},[])
  useEffect(()=>{refresh();const events=new EventSource('/api/events');events.addEventListener('message',e=>{const data=JSON.parse((e as MessageEvent).data);if(data.type==='presence.updated')setPresence(data.presence);else refresh()});return()=>events.close()},[refresh])
  useEffect(()=>{const ping=()=>api.presence({sessionId,name:'You',color:'#5546d8',selection:selected}).then(setPresence).catch(()=>{});ping();const id=setInterval(ping,15000);return()=>clearInterval(id)},[selected])
  useEffect(()=>{if(toast){const id=setTimeout(()=>setToast(''),3500);return()=>clearTimeout(id)}},[toast])

  const mutate=async(operations:Operation[])=>{if(!projectRef.current)return;try{setProject(await api.mutate(projectRef.current.revision,operations));setToast('Architecture updated')}catch(e){setToast((e as Error).message);await refresh()}}
  const review=async(id:string,decision:'accepted'|'rejected')=>{try{setProject(await api.review(id,decision,decision==='accepted'?'Reviewed against current architecture.':'Needs a narrower scope.'));setToast(decision==='accepted'?'Proposal accepted':'Proposal rejected');setProposalOpen(false)}catch(e){setToast((e as Error).message);await refresh()}}
  const breadcrumbs=useMemo(()=>{if(!project)return[];const result=[];let current=project.components.find(c=>c.id===scope);while(current){result.unshift(current);current=project.components.find(c=>c.id===current?.parentId)}return result},[project,scope])

  const canonicalNodes=useMemo<Node[]>(()=>{if(!project)return[];const view=project.views[0];const descendants=project.components.filter(c=>c.parentId===scope || (scope==='cmp_platform'&&c.id!=='cmp_platform'))
    return descendants.map((c,index)=>({id:c.id,type:'architecture',position:view?.positions[c.id]||{x:80+(index%3)*300,y:120+Math.floor(index/3)*190},data:{title:c.title,type:c.type,description:c.description,selected:c.id===selected,inferred:c.provenance.kind==='inferred',childCount:project.components.filter(n=>n.parentId===c.id).length}}))
  },[project,scope,selected])
  useEffect(()=>{if(!draggingRef.current)setFlowNodes(canonicalNodes)},[canonicalNodes,setFlowNodes])
  const visible=useMemo(()=>new Set(canonicalNodes.map(n=>n.id)),[canonicalNodes])
  const edges=useMemo<Edge[]>(()=>project?project.relationships.filter(r=>visible.has(r.sourceId)&&visible.has(r.targetId)).map(r=>({id:r.id,source:r.sourceId,target:r.targetId,label:r.label,type:'smoothstep',markerEnd:{type:MarkerType.ArrowClosed,color:'#8f8ca1'},style:{stroke:'#8f8ca1',strokeWidth:1.5},labelStyle:{fontSize:10,fill:'#6d687a'},labelBgStyle:{fill:'#f6f6f1',fillOpacity:.9}})):[],[project,visible])
  const onNodeClick:NodeMouseHandler=(_,node)=>setSelected(node.id)
  const onNodeDoubleClick:NodeMouseHandler=(_,node)=>{if(project?.components.some(c=>c.parentId===node.id))setScope(node.id)}
  const onNodeDragStart:OnNodeDrag=()=>{draggingRef.current=true}
  const onNodeDragStop:OnNodeDrag=(_,node)=>{draggingRef.current=false;void mutate([{kind:'move',targetId:node.id,component:{id:'position',type:'view',title:'position',description:'',properties:{x:String(Math.round(node.position.x)),y:String(Math.round(node.position.y))},provenance:{kind:'human',author:'You',confidence:1}}}])}
  const connect=(connection:Connection)=>{if(!connection.source||!connection.target)return;const id=`rel_${crypto.randomUUID().slice(0,8)}`;mutate([{kind:'connect',relationship:{id,sourceId:connection.source,targetId:connection.target,kind:'calls',label:'uses',provenance:{kind:'human',author:'You',confidence:1}}}])}

  if(loading)return <div className="loading-screen"><div className="brand-mark"><Waypoints/></div><span>Opening architecture workspace…</span></div>
  if(!project)return <div className="loading-screen error">Could not load the project. Start the Go API on port 8080.</div>
  const pending=project.proposals.filter(p=>p.status==='pending').length
  return <div className="app-shell">
    <nav className="rail">
      <div className="brand-mark"><Waypoints size={20}/></div>
      <div className="rail-main"><button className="active"><LayoutDashboard/></button><button><GitBranch/></button><button><MessageSquare/></button><button><History/></button></div>
      <div className="rail-bottom"><button><Bell/></button><span className="user-avatar">BS</span></div>
    </nav>
    <div className="workspace">
      <header className="topbar">
        <div className="project-switcher"><button className="sidebar-toggle"><Menu size={16}/></button><span className="project-glyph"><Box size={14}/></span><div><small>Thompson Brothers</small><button>{project.name}<ChevronDown size={12}/></button></div></div>
        <div className="top-center"><button className="search-button"><Search size={14}/><span>Search architecture</span><kbd><Command size={10}/> K</kbd></button></div>
        <div className="top-actions">
          <div className="presence-stack">{presence.slice(0,3).map((p,i)=><span key={p.sessionId} style={{zIndex:3-i,background:p.color}} title={`${p.name} is viewing ${p.selection||'the project'}`}>{p.name.slice(0,1)}</span>)}<span className="presence-static">M</span></div>
          <button className="invite"><Users size={14}/> Share</button>
          <button className={`proposal-button ${pending?'has-pending':''}`} onClick={()=>setProposalOpen(true)}><Sparkles size={14}/><span>Review</span>{pending>0&&<b>{pending}</b>}</button>
        </div>
      </header>
      <div className="content-grid">
        <Outline project={project} selected={selected} onSelect={setSelected}/>
        <main className="canvas-area">
          <div className="canvas-toolbar">
            <div className="breadcrumbs">{breadcrumbs.map((item,i)=><span key={item.id}><button onClick={()=>setScope(item.id)}>{item.title}</button>{i<breadcrumbs.length-1&&<ChevronRight size={13}/>}</span>)}</div>
            <div className="canvas-meta"><span><span className="sync-dot"/>Synced</span><span>Revision {project.revision}</span><button onClick={()=>setActivityOpen(true)}><Activity size={13}/> Activity</button></div>
          </div>
          <div className="canvas-stage">
            <ReactFlow nodes={flowNodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={onNodesChange} onConnect={connect} onNodeClick={onNodeClick} onNodeDoubleClick={onNodeDoubleClick} onNodeDragStart={onNodeDragStart} onNodeDragStop={onNodeDragStop} fitView fitViewOptions={{padding:.2}} minZoom={.3} maxZoom={1.8}>
              <Background variant={BackgroundVariant.Dots} gap={22} size={1} color="#d8d7d0"/>
              <Controls showInteractive={false}/><MiniMap pannable zoomable nodeColor={n=>n.id===selected?'#5546d8':'#d0ced8'} maskColor="rgba(246,246,241,.8)"/>
            </ReactFlow>
            <div className="floating-tools"><button className="active"><MousePointer2/></button><button title="Add component" onClick={()=>setComponentDialogOpen(true)}><Plus/></button><button title="Fit and arrange"><Grid2X2/></button><span/><button><ZoomIn/></button><button><Settings2/></button></div>
            <div className="canvas-hint">Drag between node handles to connect · double-click to explore</div>
          </div>
        </main>
        <Inspector project={project} selected={selected} onMutate={mutate}/>
      </div>
    </div>
    <ProposalDrawer project={project} open={proposalOpen} onClose={()=>setProposalOpen(false)} onReview={review}/>
    <ComponentDialog open={componentDialogOpen} project={project} parentId={scope} onClose={()=>setComponentDialogOpen(false)} onMutate={mutate}/>
    <ActivityDrawer project={project} open={activityOpen} onClose={()=>setActivityOpen(false)}/>
    {toast&&<div className="toast">{toast}</div>}
  </div>
}
