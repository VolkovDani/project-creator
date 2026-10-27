export class Latest {
 private generation=0;
 begin(){return ++this.generation}
 isCurrent(generation:number){return generation===this.generation}
 invalidate(){this.generation++}
}
