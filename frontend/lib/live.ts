import {useEffect,useRef} from 'react';
import {clientCache,clearClientSession,rejectClientSession} from './client';
import {SSEParser,retryDelay,nextCursor} from './sse';
let users=0;let controller:AbortController|undefined;let cursor:string|undefined;
const wait=(ms:number,signal:AbortSignal)=>new Promise<void>(resolve=>{if(signal.aborted){resolve();return}const done=()=>{clearTimeout(timer);signal.removeEventListener('abort',done);resolve()};const timer=setTimeout(done,ms);signal.addEventListener('abort',done,{once:true})});
async function stream(signal:AbortSignal){let attempt=0;while(!signal.aborted){try{
 const headers:Record<string,string>={'accept':'text/event-stream'};if(cursor)headers['Last-Event-ID']=cursor;
 const response=await fetch('/api/events',{headers,cache:'no-store',signal});if(signal.aborted)break;if(response.status===401||response.status===403){cursor=undefined;rejectClientSession();throw Error('unauthorized')}
 if(!response.ok||!response.body)throw Error('events unavailable');
 const reader=response.body.getReader();const decoder=new TextDecoder();const parser=new SSEParser(frame=>{if(frame.event!=='cache.invalidate'&&frame.event!=='cache.reset')return;try{const payload=JSON.parse(frame.data);if(payload.schemaVersion!==1)throw Error('unknown event');if(frame.event==='cache.reset'){clientCache.reset(false)}else {if(!Array.isArray(payload.keys)||payload.keys.some((key:unknown)=>typeof key!=='string'))throw Error('invalid keys');clientCache.invalidate(payload.keys,frame.id)}cursor=nextCursor(cursor,frame.id);attempt=0}catch{cursor=undefined;clientCache.reset(false)}});
 try{for(;;){const {done,value}=await reader.read();if(done)break;if(!signal.aborted)parser.push(decoder.decode(value,{stream:true}))}}finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
 }catch{if(signal.aborted)break}if(!signal.aborted)await wait(retryDelay(attempt++),signal)}}
function connect(){users++;if(!controller){controller=new AbortController();void stream(controller.signal)}return()=>{if(--users===0){controller?.abort();controller=undefined}}}
export function useInvalidation(tags:string[],reload:()=>void,clear:()=>void){const callbacks=useRef({reload,clear});callbacks.current={reload,clear};const key=JSON.stringify(tags);useEffect(()=>{const stop=clientCache.subscribe(JSON.parse(key),(reset)=>{if(reset)callbacks.current.clear();callbacks.current.reload()});const disconnect=connect();return()=>{stop();disconnect()}},[key])}
// Call when a future authenticated session changes; no old-session cache or cursor survives.
export function resetSession(){cursor=undefined;clearClientSession();controller?.abort();controller=undefined;if(users){controller=new AbortController();void stream(controller.signal)}}
