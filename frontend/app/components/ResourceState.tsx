'use client';
import {t,type Locale} from '../../lib/i18n';
export default function ResourceState({locale,loading,error,hasData,retry}:{locale:Locale;loading:boolean;error:boolean;hasData:boolean;retry?:()=>void}){
 return <><div className="resource-status" aria-live="polite">{error?<span role="alert" className="error">{t(locale,hasData?'ui.stale':'app.error')} {retry&&<button type="button" onClick={retry}>{t(locale,'ui.retry')}</button>}</span>:loading?<span role="status">{t(locale,hasData?'ui.refreshing':'app.loading')}</span>:null}</div>{loading&&!hasData&&<div className="skeleton" aria-hidden="true"><div/><div/><div/></div>}</>;
}
