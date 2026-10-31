import type {ProjectAnalysisStatusQuery} from './generated';

export type AnalysisRun = NonNullable<ProjectAnalysisStatusQuery['projectAnalysis']>;
export type AnalysisPairView = AnalysisRun['pairs'][number];
export type AnalysisGroupKey = 'collision' | 'compatible' | 'insufficient' | 'pending' | 'failed';
export type AnalysisGroup = {key: AnalysisGroupKey; pairs: AnalysisPairView[]};

export function groupPairs(run: AnalysisRun): AnalysisGroup[] {
 const groups: AnalysisGroup[] = ['collision','compatible','insufficient','pending','failed'].map(key=>({key:key as AnalysisGroupKey,pairs:[]}));
 const legacy: AnalysisPairView[] = run.pairs.length ? [] : run.findings.flatMap((finding,position)=>{
  const left=run.sources.find(source=>source.id===finding.leftId&&source.version===finding.leftVersion);
  const right=run.sources.find(source=>source.id===finding.rightId&&source.version===finding.rightVersion);
  return left&&right?[{id:`legacy-${position}`,position,status:'completed',left,right,verdict:finding.verdict,explanation:finding.explanation,errorCode:''}]:[];
 });
 for(const pair of [...run.pairs,...legacy].sort((a,b)=>a.position-b.position||a.id.localeCompare(b.id))){
  const key=pair.status==='pending'||pair.status==='running'?'pending':pair.status==='failed'?'failed':pair.verdict;
  groups.find(group=>group.key===key)?.pairs.push(pair);
 }
 return groups;
}

export function canRetryPairs(run: AnalysisRun): boolean {
 return run.status==='failed'&&!run.stale&&run.pairs.some(pair=>pair.status==='failed');
}
