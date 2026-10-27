import type {FieldError} from './validation';
type RequirementContract={version:number;lengthUnit:string;trim:string;fields:{text:{required:boolean;maxLength:number}}};
export function validateRequirement(c:RequirementContract,input:string){
 if(!c||Object.keys(c).sort().join()!=='fields,lengthUnit,trim,version'||c.version!==1||c.lengthUnit!=='unicode-code-points'||c.trim!=='ascii-space-tab-lf-cr'||Object.keys(c.fields).join()!=='text'||Object.keys(c.fields.text).sort().join()!=='maxLength,required'||c.fields.text.required!==true||!Number.isSafeInteger(c.fields.text.maxLength)||c.fields.text.maxLength<=0)throw Error('Unsupported requirement contract');
 const normalized=input.replace(/^[ \t\r\n]+|[ \t\r\n]+$/g,'');const errors:FieldError[]=[];let code='',messageKey='',params:Record<string,number>={};
 if(!normalized.isWellFormed()){code='INVALID_UTF8';messageKey='validation.invalidEncoding'}else if(!normalized){code='REQUIRED';messageKey='validation.required'}else if(Array.from(normalized).length>c.fields.text.maxLength){code='MAX_LENGTH';messageKey='validation.maxLength';params={max:c.fields.text.maxLength}}
 if(code)errors.push({path:['input','text'],code,messageKey,params});return {normalized,errors};
}
