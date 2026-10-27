import {NextResponse} from 'next/server';
export async function POST(request:Request) {
 const origin=request.headers.get('origin');
 if (origin) { try { if (new URL(origin).host !== request.headers.get('host')) return new NextResponse(null,{status:403}); } catch {return new NextResponse(null,{status:403})} }
 const token=process.env.PROJECT_CREATOR_TOKEN;
 const endpoint=process.env.PROJECT_CREATOR_GRAPHQL_URL || 'http://127.0.0.1:8080/graphql';
 if(!token) return NextResponse.json({errors:[{message:'configuration error'}]},{status:503});
 try {
  const text=await request.text(); if(Buffer.byteLength(text)>1048576)return new NextResponse(null,{status:413});
  const response=await fetch(endpoint,{method:'POST',headers:{'content-type':'application/json','authorization':`Bearer ${token}`},body:text,cache:'no-store',signal:AbortSignal.timeout(10000)});
  return new NextResponse(await response.text(),{status:response.status,headers:{'content-type':'application/json','cache-control':'no-store'}});
 } catch {return NextResponse.json({errors:[{message:'backend unavailable'}]},{status:502})}
}
