import contract from '../../contracts/analysis.json' with {type:'json'};
import type {FieldError} from './validation.ts';

// This JSON is also embedded by Go; forms do not own separate threshold rules.
export const analysisRules=Object.freeze({...contract,modes:Object.freeze([...contract.modes])});
if(analysisRules.version!==1||!analysisRules.modes.includes(analysisRules.defaultMode)||analysisRules.minThreshold>analysisRules.maxThreshold||analysisRules.defaultThreshold<analysisRules.minThreshold||analysisRules.defaultThreshold>analysisRules.maxThreshold)throw Error('Unsupported analysis contract');

export function validateAnalysisSelection(input:{mode?:string|null;threshold?:number|null}){
 const normalized={mode:input.mode??analysisRules.defaultMode,threshold:input.threshold??analysisRules.defaultThreshold};
 const errors:FieldError[]=[];
 if(!analysisRules.modes.includes(normalized.mode))errors.push({path:['mode'],code:'INVALID_CHOICE',messageKey:'analysis.validation.mode',params:{}});
 if(!Number.isFinite(normalized.threshold)||normalized.threshold<analysisRules.minThreshold||normalized.threshold>analysisRules.maxThreshold)errors.push({path:['threshold'],code:'RANGE',messageKey:'analysis.validation.threshold',params:{min:analysisRules.minThreshold,max:analysisRules.maxThreshold}});
 return {normalized,errors};
}
