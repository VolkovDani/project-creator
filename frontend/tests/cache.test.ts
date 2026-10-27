import {test} from 'node:test';
import assert from 'node:assert/strict';
import {TaggedCache} from '../lib/cache.ts';
import {SSEParser,nextCursor} from '../lib/sse.ts';
test('tag invalidation clears matching entries and duplicate ids are ignored',async()=>{
 const c=new TaggedCache();let loads=0,events=0;c.subscribe(['projects'],()=>events++);
 const load=async()=>({data:++loads,tags:['projects']});
 assert.equal(await c.read('list',load),1);assert.equal(await c.read('list',load),1);
 c.invalidate(['projects'],'a');assert.equal(events,1);assert.equal(await c.read('list',load),2);
 c.invalidate(['projects'],'a');assert.equal(events,1);assert.equal(await c.read('list',load),2);
 c.invalidate(['other'],'b');assert.equal(events,1);
});
test('invalidation during fetch repeats it before returning',async()=>{
 const c=new TaggedCache();let release!:()=>void;const pending=new Promise<void>(r=>release=r);let loads=0;
 const result=c.read('q',async()=>{const data=++loads;if(data===1)await pending;return {data,tags:['projects']}});
 c.invalidate(['projects'],'one');release();assert.equal(await result,2);
});
test('reset clears all entries, cursor dedup and notifies subscribers',async()=>{
 const c=new TaggedCache();let n=0;c.subscribe(['x'],()=>n++);await c.read('q',async()=>({data:1,tags:['x']}));
 c.invalidate(['x'],'old');c.reset();c.invalidate(['x'],'old');assert.equal(n,3);
});
test('SSE parser handles split CRLF, comments, multiline and cursor',()=>{
 const frames:unknown[]=[];const parser=new SSEParser(frame=>frames.push(frame));
 parser.push(': heartbeat\r');parser.push('\nid: cursor-1\r\nevent: cache.invalidate\r\ndata: {"keys":\r\ndata: ["projects"]}\r\n\r\n');
 assert.deepEqual(frames,[{id:'cursor-1',event:'cache.invalidate',data:'{"keys":\n["projects"]}'}]);
});
test('unrelated cache entries survive tagged invalidation',async()=>{
 const c=new TaggedCache();let n=0;const load=async()=>({data:++n,tags:['other']});await c.read('other',load);c.invalidate(['projects'],'x');assert.equal(await c.read('other',load),1);
});
test('unsubscribe stops callbacks',()=>{const c=new TaggedCache();let count=0;const stop=c.subscribe(['x'],()=>count++);stop();c.reset();assert.equal(count,0)});

test('empty SSE id clears reconnect cursor',()=>{assert.equal(nextCursor('old',''),undefined);assert.equal(nextCursor('old',undefined),'old');assert.equal(nextCursor('old','new'),'new')});
test('cache reset refreshes subscriptions without clearing unsaved UI state',()=>{const c=new TaggedCache();const modes:boolean[]=[];c.subscribe(['projects'],clear=>modes.push(clear));c.reset(false);c.reset();assert.deepEqual(modes,[false,true])});
