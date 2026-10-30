'use client';
import {useCallback,useEffect,useRef,useState} from 'react';
import {query,clientCache} from '../../../lib/client';
import {Latest} from '../../../lib/latest';
import {useInvalidation} from '../../../lib/live';
import {requirementAnchor} from '../../../lib/rag';
import {groupPairs,canRetryPairs,type AnalysisRun,type AnalysisGroupKey,type AnalysisPairView} from '../../../lib/analysis';
import {t,type Locale} from '../../../lib/i18n';
import {ProjectAnalysisStatusDocument,StartProjectAnalysisDocument,RetryProjectAnalysisDocument} from '../../../lib/generated';

const outcomes:Record<string,string>={possible_collision:'analysis.outcome.collision',insufficient:'analysis.outcome.insufficient',no_collisions_found:'analysis.outcome.none'};
const reasonCodes=new Set(['INDEX_INCOMPLETE','TOO_FEW_REQUIREMENTS','REQUIREMENT_LIMIT','ANALYSIS_LIMIT','NO_CANDIDATES','MODEL_INSUFFICIENT']);
const verdicts:Record<string,string>={collision:'analysis.verdict.collision',insufficient:'analysis.verdict.insufficient',compatible:'analysis.verdict.compatible'};
const stages=new Set(['checking_index','finding_pairs','comparing']);
type Source=AnalysisPairView['left'];
type SourceHandler=(id:string)=>Promise<void>;

function SavedSource({source,locale,onSource}:{source:Source;locale:Locale;onSource?:SourceHandler}){
 return <div className="analysis-source">
  <p className="muted">{source.id} · {t(locale,'requirements.version',{version:source.version})}</p>
  <p>{source.text}</p>
  <a href={requirementAnchor(source.id)} onClick={onSource?event=>{event.preventDefault();void onSource(source.id)}:undefined}>{t(locale,'rag.source')}</a>
 </div>;
}

export default function Analysis({projectId,locale,onSource}:{projectId:string;locale:Locale;onSource?:SourceHandler}){
 const [run,setRun]=useState<AnalysisRun|null>();
 const [busy,setBusy]=useState(false);
 const [error,setError]=useState(false);
 const [expanded,setExpanded]=useState<Record<AnalysisGroupKey,boolean>>({collision:true,compatible:true,insufficient:true,pending:true,failed:true});
 const latest=useRef(new Latest());
 const operation=useRef(0);
 const context=useRef(projectId);
 const active=useRef(true);
 if(context.current!==projectId){context.current=projectId;operation.current++;latest.current.invalidate()}
 const load=useCallback(async()=>{
  const generation=latest.current.begin();
  const epoch=operation.current;
  try{
   const data=await query(ProjectAnalysisStatusDocument,{projectId});
   if(active.current&&context.current===projectId&&operation.current===epoch&&latest.current.isCurrent(generation)){setRun(data.projectAnalysis);setError(false)}
  }catch{if(active.current&&operation.current===epoch&&latest.current.isCurrent(generation))setError(true)}
 },[projectId]);
 useEffect(()=>{
  active.current=true;operation.current++;setRun(undefined);setBusy(false);setError(false);void load();
  return()=>{active.current=false;operation.current++;latest.current.invalidate()};
 },[load]);
 useInvalidation([`project:${projectId}`,`project:${projectId}:requirements`],()=>void load(),()=>{
  operation.current++;latest.current.invalidate();setRun(undefined);setBusy(false);setError(false);
 });
 async function start(retry=false){
  const epoch=++operation.current;
  latest.current.invalidate();setBusy(true);setError(false);
  try{
   const next=retry&&run
    ?(await query(RetryProjectAnalysisDocument,{projectId,analysisId:run.id})).retryProjectAnalysis
    :(await query(StartProjectAnalysisDocument,{projectId})).startProjectAnalysis;
   if(active.current&&context.current===projectId&&operation.current===epoch){latest.current.invalidate();setRun(next);void load()}
  }catch{if(active.current&&context.current===projectId&&operation.current===epoch)setError(true)}
  finally{if(active.current&&context.current===projectId&&operation.current===epoch)setBusy(false)}
 }
 const heading=`analysis-heading-${encodeURIComponent(projectId)}`;
 const status=busy?'running':run?.status;
 const statusKey=status==='running'?'analysis.status.running':status==='failed'?'analysis.status.failed':status==='completed'?'analysis.status.completed':undefined;
 const groups=run?groupPairs(run):[];
 const retryable=!!run&&canRetryPairs(run);
 const candidatesKnown=!!run&&(run.totalPairs!=null||run.pairs.length>0||run.findings.length>0);
 return <section role="region" aria-labelledby={heading}>
  <h2 id={heading}>{t(locale,'analysis.title')}</h2>
  <p>{t(locale,'analysis.help')}</p>
  <p>{t(locale,'analysis.disclaimer')}</p>
  <button type="button" disabled={busy||run?.status==='running'} onClick={()=>void start()}>{t(locale,busy?'analysis.starting':run?'analysis.newRun':'analysis.start')}</button>
  {run?.status==='running'&&<button type="button" disabled={busy} onClick={()=>clientCache.invalidate([`project:${projectId}`])}>{t(locale,'analysis.refresh')}</button>}
  {retryable&&<button type="button" disabled={busy} onClick={()=>void start(true)}>{t(locale,'analysis.retryFailed')}</button>}
  {statusKey&&<p role="status">{t(locale,statusKey)}</p>}
  {error&&<p role="alert" className="error">{t(locale,'analysis.requestFailed')}</p>}
  {run?.stale&&<p role="alert">{t(locale,'analysis.stale')}</p>}
  {run?.status==='running'&&stages.has(run.stage)&&<p role="status">{t(locale,`analysis.stage.${run.stage}`)}</p>}
  {run&&run.totalPairs!=null&&<p role="status">{t(locale,'analysis.progress',{processed:run.processedPairs,total:run.totalPairs})}</p>}
  {run?.status==='completed'&&<h3>{t(locale,outcomes[run.outcome]||'analysis.outcome.insufficient')}</h3>}
  {run?.status==='completed'&&reasonCodes.has(run.reasonCode)&&<p role="status">{t(locale,`analysis.reason.${run.reasonCode}`)}</p>}
  {run?.status==='completed'&&run.reasonCode==='INDEX_INCOMPLETE'&&run.missingRequirementIds.length>0&&<div role="group" aria-label={t(locale,'analysis.missingIndexes',{count:run.missingRequirementIds.length})}>
   <h3>{t(locale,'analysis.missingIndexes',{count:run.missingRequirementIds.length})}</h3>
   <ul>{run.missingRequirementIds.map(id=><li key={id}><a href={requirementAnchor(id)} onClick={onSource?event=>{event.preventDefault();void onSource(id)}:undefined}>{id}</a></li>)}</ul>
  </div>}
  {run?.status==='failed'&&<p>{t(locale,'analysis.failureHelp')}</p>}
  {retryable&&<p>{t(locale,'analysis.retryFailedHelp')}</p>}
  {run&&<>
   {run.sources.length>0&&<h3>{t(locale,'analysis.sources')}</h3>}
   {candidatesKnown&&groups.map(group=><details className="analysis-group" key={group.key} open={expanded[group.key]} aria-label={t(locale,`analysis.group.${group.key}`)} onToggle={event=>{
    const open=event.currentTarget.open;
    setExpanded(previous=>previous[group.key]===open?previous:{...previous,[group.key]:open});
   }}>
    <summary>{t(locale,'analysis.group.count',{name:t(locale,`analysis.group.${group.key}`),count:group.pairs.length})}</summary>
    {group.pairs.length===0&&<p className="muted">{t(locale,'analysis.group.empty')}</p>}
    {group.pairs.map(pair=><article className={`card analysis-pair${pair.status==='running'?' analysis-pair-current':''}`} key={pair.id} data-pair-id={pair.id} aria-label={t(locale,'analysis.pair.position',{number:pair.position+1})}>
     <h4>{t(locale,'analysis.pair.position',{number:pair.position+1})}</h4>
     {pair.status==='completed'&&<p>{t(locale,verdicts[pair.verdict]||'analysis.verdict.insufficient')}</p>}
     {(pair.status==='pending'||pair.status==='running')&&<p role="status">{t(locale,`analysis.pair.${pair.status}`)}</p>}
     {pair.status==='failed'&&<p className="error">{t(locale,pair.errorCode==='NOT_PROCESSED'?'analysis.pair.notProcessed':'analysis.pair.failed')}</p>}
     <SavedSource source={pair.left} locale={locale} onSource={onSource}/>
     <SavedSource source={pair.right} locale={locale} onSource={onSource}/>
     {pair.explanation&&<p>{pair.explanation}</p>}
    </article>)}
   </details>)}
   {groups.every(group=>group.pairs.length===0)&&run.sources.map(source=><SavedSource key={`${source.id}:${source.version}`} source={source} locale={locale} onSource={onSource}/>)}
  </>}
 </section>;
}
