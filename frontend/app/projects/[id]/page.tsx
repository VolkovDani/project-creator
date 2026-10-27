'use client';
import Link from 'next/link';
import {useParams} from 'next/navigation';
import {useEffect,useState} from 'react';
import {ProjectDetailDocument,type ProjectDetailQuery} from '../../../lib/generated';
import {query} from '../../../lib/client';
import {t,type Locale} from '../../../lib/i18n';
import Requirements from './Requirements';
export default function ProjectPage(){const {id}=useParams<{id:string}>();const [locale,setLocale]=useState<Locale>('ru');const [project,setProject]=useState<ProjectDetailQuery['project']>();const [loading,setLoading]=useState(true);const [error,setError]=useState(false);useEffect(()=>{let active=true;query(ProjectDetailDocument,{id}).then(d=>{if(active)setProject(d.project)}).catch(()=>{if(active)setError(true)}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[id]);return <main><div className="top"><Link href="/">{t(locale,'app.back')}</Link><label>{t(locale,'app.language')} <select value={locale} onChange={e=>setLocale(e.target.value as Locale)}><option value="ru">Русский</option><option value="en">English</option></select></label></div>{loading?<p>{t(locale,'app.loading')}</p>:error?<p className="error">{t(locale,'app.error')}</p>:!project?<p>{t(locale,'app.notFound')}</p>:<section><h1>{project.name}</h1><h2>{t(locale,'app.goal')}</h2><p>{project.goal}</p><h2>{t(locale,'app.description')}</h2><p>{project.description}</p></section>}{project&&<Requirements projectId={project.id} locale={locale}/>}</main>}
