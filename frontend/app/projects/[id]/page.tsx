'use client';
import Link from 'next/link';
import {useParams} from 'next/navigation';
import {useCallback,useEffect,useRef,useState} from 'react';
import {Latest} from '../../../lib/latest';
import {useInvalidation} from '../../../lib/live';
import {ProjectDetailDocument,type ProjectDetailQuery} from '../../../lib/generated';
import {query} from '../../../lib/client';
import {t,type Locale} from '../../../lib/i18n';
import Requirements from './Requirements';
export default function ProjectPage(){const {id}=useParams<{id:string}>();const [locale,setLocale]=useState<Locale>('ru');const [project,setProject]=useState<ProjectDetailQuery['project']>();const [loading,setLoading]=useState(true);const [error,setError]=useState(false);const latest=useRef(new Latest());const load=useCallback(async()=>{const generation=latest.current.begin();setLoading(true);setError(false);try{const data=await query(ProjectDetailDocument,{id});if(latest.current.isCurrent(generation))setProject(data.project)}catch{if(latest.current.isCurrent(generation))setError(true)}finally{if(latest.current.isCurrent(generation))setLoading(false)}},[id]);useInvalidation([`project:${id}`],()=>void load(),()=>setProject(undefined));useEffect(()=>{void load();const state=latest.current;return()=>state.invalidate()},[load]);return <main><div className="top"><Link href="/">{t(locale,'app.back')}</Link><label>{t(locale,'app.language')} <select value={locale} onChange={e=>setLocale(e.target.value as Locale)}><option value="ru">Русский</option><option value="en">English</option></select></label></div>{loading?<p>{t(locale,'app.loading')}</p>:error?<p className="error">{t(locale,'app.error')}</p>:!project?<p>{t(locale,'app.notFound')}</p>:<section><h1>{project.name}</h1><h2>{t(locale,'app.goal')}</h2><p>{project.goal}</p><h2>{t(locale,'app.description')}</h2><p>{project.description}</p></section>}{project&&<Requirements projectId={project.id} locale={locale}/>}</main>}

