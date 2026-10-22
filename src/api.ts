import type { DiscoveryReport, Operation, Presence, Project } from './types'

async function request<T>(path:string, options?:RequestInit):Promise<T> {
  const response = await fetch(path, { headers:{'Content-Type':'application/json'}, ...options })
  const data = await response.json()
  if (!response.ok) throw new Error(data.error || `Request failed (${response.status})`)
  return data
}

export const api = {
  project: () => request<Project>('/api/project'),
  mutate: (baseRevision:number, operations:Operation[]) => request<Project>('/api/mutations', {method:'POST', body:JSON.stringify({baseRevision, author:'You', operations})}),
  review: (id:string, decision:'accepted'|'rejected', comment:string) => request<Project>(`/api/proposals/${id}/review`, {method:'POST', body:JSON.stringify({reviewer:'You',decision,comment})}),
  presence: (presence:Omit<Presence,'lastSeen'>) => request<Presence[]>('/api/presence', {method:'POST',body:JSON.stringify(presence)}),
  discover: (path:string,maxFiles=25000) => request<DiscoveryReport>('/api/discovery', {method:'POST',body:JSON.stringify({path,maxFiles})}),
}
