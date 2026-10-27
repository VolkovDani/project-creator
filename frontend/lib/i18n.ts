import ru from '../../contracts/i18n/ru.json';
import en from '../../contracts/i18n/en.json';
export type Locale='ru'|'en';
const dictionaries={ru,en};
export function t(locale:Locale,key:string,params:Record<string,number|string>={}):string {
 const dict:Record<string,string>=dictionaries[locale];const template=dict[key]||dict['app.validationUnknown'];
 return template.replace(/\{(\w+)\}/g,(_,k:string)=>String(params[k]??''));
}
