import {test} from 'node:test';
import assert from 'node:assert/strict';
import {groupPairs,canRetryPairs,type AnalysisRun,type AnalysisPairView} from '../lib/analysis.ts';
import fs from 'node:fs';

const left={id:'left',version:1,text:'First saved source'};
const right={id:'right',version:2,text:'Second saved source'};
const pair=(id:string,position:number,status:string,verdict=''):AnalysisPairView=>({id,position,status,left,right,verdict,explanation:verdict?'An explanation':'',errorCode:''});
const run=(pairs:AnalysisPairView[],overrides:Partial<AnalysisRun>={}):AnalysisRun=>({id:'run',projectId:'project',status:'running',stage:'comparing',totalPairs:pairs.length,processedPairs:0,activePairId:'',outcome:'',errorCode:'',reasonCode:'',missingRequirementIds:[],createdAt:'now',completedAt:'',stale:false,generatorProfile:'g',embeddingProfile:'e',sources:[left,right],findings:[],pairs,...overrides});

test('pairs are grouped by verdict and processing state with stable candidate order',()=>{
 const value=run([pair('later',3,'completed','collision'),pair('running',4,'running'),pair('earlier',1,'completed','collision'),pair('compatible',2,'completed','compatible'),pair('pending',5,'pending'),pair('insufficient',6,'completed','insufficient'),pair('failed',7,'failed')]);
 const groups=groupPairs(value);
 assert.deepEqual(groups.map(group=>group.key),['collision','compatible','insufficient','pending','failed']);
 assert.deepEqual(groups.map(group=>group.pairs.map(item=>item.id)),[['earlier','later'],['compatible'],['insufficient'],['running','pending'],['failed']]);
 assert.equal(value.pairs[0].id,'later','grouping does not mutate the API snapshot');
});

test('a completed pair moves out of pending without duplicating its card',()=>{
 const checking=pair('pair',0,'running');
 const before=groupPairs(run([checking]));
 assert.equal(before.find(group=>group.key==='pending')?.pairs.length,1);
 const after=groupPairs(run([{...checking,status:'completed',verdict:'compatible',explanation:'No contradiction found'}]));
 assert.equal(after.find(group=>group.key==='pending')?.pairs.length,0);
 assert.equal(after.find(group=>group.key==='compatible')?.pairs.length,1);
 assert.equal(after.flatMap(group=>group.pairs).length,1);
});

test('legacy findings keep the saved source texts and exact versions',()=>{
 const groups=groupPairs(run([],{status:'completed',findings:[{leftId:'left',rightId:'right',leftVersion:1,rightVersion:2,verdict:'collision',explanation:'A legacy finding'}]}));
 assert.equal(groups[0].pairs.length,1);
 assert.deepEqual(groups[0].pairs[0].left,left);
 assert.deepEqual(groups[0].pairs[0].right,right);
 assert.equal(groups[0].pairs[0].explanation,'A legacy finding');
});

test('only a current stopped run with failed pairs can resume failed work',()=>{
 const failed=run([pair('failed',0,'failed')],{status:'failed',stage:'finished'});
 assert.equal(canRetryPairs(failed),true);
 assert.equal(canRetryPairs({...failed,stale:true}),false);
 assert.equal(canRetryPairs({...failed,status:'running'}),false);
 assert.equal(canRetryPairs(run([pair('done',0,'completed','collision')],{status:'completed'})),false);
});

test('analysis translations have matching keys and interpolation parameters',()=>{
 const ru=JSON.parse(fs.readFileSync(new URL('../../contracts/i18n/ru.json',import.meta.url),'utf8')) as Record<string,string>;
 const en=JSON.parse(fs.readFileSync(new URL('../../contracts/i18n/en.json',import.meta.url),'utf8')) as Record<string,string>;
 const keys=(dict:Record<string,string>)=>Object.keys(dict).filter(key=>key.startsWith('analysis.')).sort();
 assert.deepEqual(keys(ru),keys(en));
 for(const key of keys(ru)){
  const params=(text:string)=>[...text.matchAll(/\{(\w+)\}/g)].map(match=>match[1]).sort();
  assert.deepEqual(params(ru[key]),params(en[key]),key);
 }
 assert.equal(ru['analysis.progress'],'Обработано {processed} из {total}');
 assert.equal(en['analysis.progress'],'Processed {processed} of {total}');
});
