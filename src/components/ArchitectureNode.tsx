import { Handle, Position, type NodeProps } from '@xyflow/react'
import { Box, Braces, Database, Globe2, Server, ShieldCheck, Sparkles } from 'lucide-react'

type Data = { title:string; type:string; description:string; selected:boolean; inferred:boolean; childCount:number }
const icons:Record<string,typeof Box> = { system:Box, client:Globe2, service:Server, database:Database, api:Braces, security:ShieldCheck }

export function ArchitectureNode({data}:NodeProps) {
  const d = data as Data
  const Icon = icons[d.type] || Box
  return <div className={`architecture-node type-${d.type} ${d.selected?'selected':''}`}>
    <Handle type="target" position={Position.Left} />
    <div className="node-head">
      <span className="node-icon"><Icon size={15}/></span>
      <span className="node-type">{d.type}</span>
      {d.inferred && <span className="node-ai" title="Agent-proposed or inferred"><Sparkles size={11}/></span>}
      <span className="node-menu">•••</span>
    </div>
    <div className="node-title">{d.title}</div>
    <div className="node-description">{d.description}</div>
    <div className="node-foot"><span className="status-dot"/> Healthy {d.childCount>0 && <b>{d.childCount} inside</b>}</div>
    <Handle type="source" position={Position.Right} />
  </div>
}
