import {print} from 'graphql';
import type {TypedDocumentNode} from '@graphql-typed-document-node/core';
import {TaggedCache} from './cache.ts';
export const clientCache=new TaggedCache();
let unauthorized=false;
function fallbackTags(variables:unknown){const v=variables as {id?:string;projectId?:string;input?:{projectId?:string}};const id=v.projectId||v.input?.projectId||v.id;return ['projects',...(id?[`project:${id}`,`project:${id}:requirements`]:[])]}
export async function query<T,V>(document:TypedDocumentNode<T,V>,variables:V):Promise<T> {
 const source=print(document);const operation=document.definitions.find(def=>def.kind==='OperationDefinition');const mutation=operation?.kind==='OperationDefinition'&&operation.operation==='mutation';
 const load=async()=>{
  const response=await fetch('/api/graphql',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({query:source,variables}),cache:'no-store'});
  if(response.status===401||response.status===403){if(!unauthorized){unauthorized=true;clientCache.reset()}throw Error('unauthorized')}
  if(!response.ok)throw Error('request failed');unauthorized=false;const payload=await response.json();if(payload.errors?.length||!payload.data)throw Error('graphql error');
  const tags=Array.isArray(payload.extensions?.cacheTags)?payload.extensions.cacheTags.filter((tag:unknown)=>typeof tag==='string'):fallbackTags(variables);
  return {data:payload.data as T,tags};
 };
 if(mutation){const result=await load();clientCache.invalidate(result.tags);return result.data}
 return clientCache.read(JSON.stringify([source,variables]),load);
}
