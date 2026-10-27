export type Input = {name:string;goal:string;description:string};
export type FieldError = {path:string[];code:string;messageKey:string;params:Record<string,number>};
export type Contract = {version:number;lengthUnit:string;trim:string;fields:Record<keyof Input,{required:boolean;maxLength:number}>};
export function validateContract(c:Contract) {
 if(!c || Object.keys(c).sort().join()!=='fields,lengthUnit,trim,version' || c.version!==1 || c.lengthUnit!=='unicode-code-points' || c.trim!=='ascii-space-tab-lf-cr' || Object.keys(c.fields).sort().join()!=='description,goal,name') throw Error('Unsupported contract');
 for(const field of ['name','goal','description'] as const){const rule=c.fields[field];if(!rule || Object.keys(rule).sort().join()!=='maxLength,required' || rule.required!==(field!=='description') || !Number.isSafeInteger(rule.maxLength)||rule.maxLength<=0)throw Error('Unsupported field constraint')}
}
export function validate(c:Contract,input:Input) {
 validateContract(c);
 const normalized:Input={...input};const errors:FieldError[]=[];
 for(const field of ['name','goal','description'] as const){
  const value=input[field].replace(/^[ \t\r\n]+|[ \t\r\n]+$/g,'');normalized[field]=value;const rule=c.fields[field];let code='',messageKey='',params:Record<string,number>={};
  if(!value.isWellFormed()){code='INVALID_UTF8';messageKey='validation.invalidEncoding'}
  else if(rule.required && value===''){code='REQUIRED';messageKey='validation.required'}
  else if(Array.from(value).length>rule.maxLength){code='MAX_LENGTH';messageKey='validation.maxLength';params={max:rule.maxLength}}
  if(code) errors.push({path:['input',field],code,messageKey,params});
 }
 return {normalized,errors};
}
