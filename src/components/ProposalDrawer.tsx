import { ArrowRight, Check, GitPullRequest, Plus, Sparkles, X } from 'lucide-react'
import type { Project, Proposal } from '../types'

type Props={project:Project; open:boolean; onClose:()=>void; onReview:(id:string,d:'accepted'|'rejected')=>Promise<void>}
export default function ProposalDrawer({project,open,onClose,onReview}:Props){
  const proposal=project.proposals.find(p=>p.status==='pending')||project.proposals[0]
  if(!open||!proposal)return null
  return <div className="drawer-backdrop" onMouseDown={e=>{if(e.target===e.currentTarget)onClose()}}><aside className="proposal-drawer">
    <div className="drawer-top"><div><span className="proposal-kicker"><Sparkles size={13}/> Agent proposal</span><h2>{proposal.title}</h2></div><button className="icon-button" onClick={onClose}><X size={17}/></button></div>
    <div className="proposal-author"><span className="atlas-avatar">A</span><div><b>{proposal.author}</b><span>submitted against revision {proposal.baseRevision}</span></div><span className={`proposal-status ${proposal.status}`}>{proposal.status}</span></div>
    <p className="proposal-summary">{proposal.summary}</p>
    <div className="diff-heading"><span>Proposed operations</span><b>{proposal.operations.length}</b></div>
    <div className="operations">{proposal.operations.map((op,i)=><OperationRow key={i} op={op}/>)}</div>
    <div className="trust-note"><GitPullRequest size={16}/><p><b>Review boundary</b><br/>Nothing here changes the canonical graph until this proposal is accepted.</p></div>
    {proposal.status==='pending'?<div className="review-actions"><button className="reject" onClick={()=>onReview(proposal.id,'rejected')}><X size={15}/> Reject</button><button className="accept" onClick={()=>onReview(proposal.id,'accepted')}><Check size={15}/> Accept {proposal.operations.length} changes</button></div>:<div className="reviewed-state"><Check size={17}/> Reviewed by {proposal.reviewer}</div>}
  </aside></div>
}
function OperationRow({op}:{op:Proposal['operations'][number]}){const title=op.component?.title||op.block?.title||op.relationship?.label||op.targetId;return <div className="operation-row"><span className="op-icon"><Plus size={13}/></span><div><span>{op.kind.replace('_',' ')}</span><b>{title}</b>{op.relationship&&<small>{op.relationship.sourceId}<ArrowRight size={11}/>{op.relationship.targetId}</small>}</div></div>}
