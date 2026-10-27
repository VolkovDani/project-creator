export type Frame={id?:string;event:string;data:string};
export class SSEParser {
 private buffer='';private data:string[]=[];private event='message';private id:string|undefined;
 constructor(privateFrame:(frame:Frame)=>void){this.emit=privateFrame}
 private emit:(frame:Frame)=>void;
 push(chunk:string){this.buffer+=chunk;for(;;){const match=/\r\n|\r|\n/.exec(this.buffer);if(!match)return;if(match[0]==='\r'&&match.index===this.buffer.length-1)return;const line=this.buffer.slice(0,match.index);this.buffer=this.buffer.slice(match.index+match[0].length);this.line(line)}}
 private line(line:string){
  if(!line){if(this.data.length)this.emit({id:this.id,event:this.event,data:this.data.join('\n')});this.data=[];this.event='message';this.id=undefined;return}
  if(line.startsWith(':'))return;const colon=line.indexOf(':');const field=colon<0?line:line.slice(0,colon);let value=colon<0?'':line.slice(colon+1);if(value.startsWith(' '))value=value.slice(1);
  if(field==='data')this.data.push(value);if(field==='event')this.event=value;if(field==='id'&&!value.includes('\0'))this.id=value;
 }
}
export function retryDelay(attempt:number){return Math.min(30000,500*2**Math.min(attempt,6))}

export function nextCursor(current:string|undefined,id:string|undefined){return id===undefined?current:id||undefined}
