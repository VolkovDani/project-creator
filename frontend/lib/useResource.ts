'use client';
import {useCallback,useEffect,useRef,useState} from 'react';
import {Latest} from './latest';
import {useInvalidation} from './live';

// Keep successful data during refresh. Changing identity unmounts consumers via their key.
export function useResource<T>(loadData:()=>Promise<T>,tags:string[]){
 const [data,setData]=useState<T>();
 const [loading,setLoading]=useState(true);
 const [error,setError]=useState(false);
 const latest=useRef(new Latest());
 const load=useCallback(async()=>{
  const generation=latest.current.begin();setLoading(true);setError(false);
  try{const result=await loadData();if(latest.current.isCurrent(generation))setData(result)}
  catch{if(latest.current.isCurrent(generation))setError(true)}
  finally{if(latest.current.isCurrent(generation))setLoading(false)}
 },[loadData]);
 useInvalidation(tags,()=>void load(),()=>{latest.current.invalidate();setData(undefined);setError(false)});
 useEffect(()=>{void load();const state=latest.current;return()=>state.invalidate()},[load]);
 return {data,loading,error,reload:load};
}
