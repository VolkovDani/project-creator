type Entry={data:unknown;tags:string[]};
type Listener={tags:string[];callback:(reset:boolean)=>void};
export class TaggedCache {
 private entries=new Map<string,Entry>();private listeners=new Set<Listener>();private generation=0;private seen=new Set<string>();
 subscribe(tags:string[],callback:(reset:boolean)=>void){const item={tags,callback};this.listeners.add(item);return()=>{this.listeners.delete(item)}}
 async read<T>(key:string,load:()=>Promise<{data:T;tags:string[]}>):Promise<T>{
  const cached=this.entries.get(key);if(cached)return cached.data as T;
  for(;;){const generation=this.generation;const result=await load();if(generation!==this.generation)continue;this.entries.set(key,result);return result.data}
 }
 invalidate(tags:string[],id?:string){
  if(id&&this.seen.has(id))return;if(id){this.seen.add(id);if(this.seen.size>1024)this.seen.delete(this.seen.values().next().value!)}
  this.generation++;const intersects=(values:string[])=>values.some(tag=>tags.includes(tag));
  for(const [key,value] of this.entries)if(intersects(value.tags))this.entries.delete(key);
  for(const listener of this.listeners)if(intersects(listener.tags))listener.callback(false);
 }
 reset(clear=true){this.generation++;this.entries.clear();this.seen.clear();for(const listener of this.listeners)listener.callback(clear)}
}
