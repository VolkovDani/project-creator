import {NextResponse} from 'next/server';
export const dynamic='force-dynamic';
export async function GET(request:Request){
 const origin=request.headers.get('origin');if(origin){try{if(new URL(origin).host!==request.headers.get('host'))return new NextResponse(null,{status:403})}catch{return new NextResponse(null,{status:403})}}
 if(request.headers.get('sec-fetch-site')==='cross-site')return new NextResponse(null,{status:403});
 const token=process.env.PROJECT_CREATOR_TOKEN;if(!token)return new NextResponse(null,{status:503});
 const endpoint=new URL(process.env.PROJECT_CREATOR_GRAPHQL_URL||'http://127.0.0.1:8080/graphql');endpoint.pathname='/events';endpoint.search='';
 const abort=new AbortController();request.signal.addEventListener('abort',()=>abort.abort(),{once:true});
 const headers:Record<string,string>={'authorization':`Bearer ${token}`,'accept':'text/event-stream'};const cursor=request.headers.get('last-event-id');if(cursor)headers['Last-Event-ID']=cursor;
 try{const response=await fetch(endpoint,{headers,cache:'no-store',signal:abort.signal});if(!response.ok||!response.body){abort.abort();return new NextResponse(null,{status:response.status})}
 const reader=response.body.getReader();const body=new ReadableStream({async pull(controller){try{const {done,value}=await reader.read();if(done)controller.close();else controller.enqueue(value)}catch(error){controller.error(error);abort.abort()}},async cancel(){abort.abort();await reader.cancel().catch(()=>{})}});
 return new NextResponse(body,{headers:{'content-type':'text/event-stream','cache-control':'no-store, no-transform','x-accel-buffering':'no'}})
 }catch{return new NextResponse(null,{status:502})}
}
