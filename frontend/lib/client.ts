import {print} from 'graphql';
import type {TypedDocumentNode} from '@graphql-typed-document-node/core';
export async function query<T,V>(document:TypedDocumentNode<T,V>,variables:V):Promise<T> {
 const response=await fetch('/api/graphql',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({query:print(document),variables}),cache:'no-store'});
 if(!response.ok)throw Error('request failed');const payload=await response.json();if(payload.errors?.length||!payload.data)throw Error('graphql error');return payload.data as T;
}
