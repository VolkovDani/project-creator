export function indexStatus(index:{version:number;status:string}|undefined,currentVersion:number):'pending'|'ready'|'failed'{if(!index||index.version!==currentVersion)return 'pending';return index.status==='ready'?'ready':index.status==='failed'?'failed':'pending'}
export function requirementAnchor(id:string){return `#requirement-${encodeURIComponent(id)}`}

export function requirementURL(projectId:string,id:string){return `/projects/${encodeURIComponent(projectId)}/requirements?requirement=${encodeURIComponent(id)}${requirementAnchor(id)}`}
