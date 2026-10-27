import {test} from 'node:test';
import assert from 'node:assert/strict';
import {parse} from 'graphql';
import {clientCache,query} from '../lib/client.ts';
test('unauthorized responses clear state once instead of recursive refetch resets',async()=>{
 const original=globalThis.fetch;let resets=0;const stop=clientCache.subscribe(['projects'],clear=>{if(clear)resets++});
 try{globalThis.fetch=async()=>new Response(null,{status:401});const document=parse('{ projects(limit:20,offset:0){id} }');await assert.rejects(query(document,{}),/unauthorized/);await assert.rejects(query(document,{}),/unauthorized/);assert.equal(resets,1)}finally{globalThis.fetch=original;stop()}
});
