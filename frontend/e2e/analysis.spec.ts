import {test,expect,type Page} from '@playwright/test';
const sources=[{id:'left',version:1,text:'Оплата разрешена только картой.'},{id:'right',version:2,text:'Оплата разрешена только наличными.'}];
const complete=(overrides:Record<string,unknown>={})=>({id:'run-1',projectId:'demo',status:'completed',outcome:'possible_collision',errorCode:'',createdAt:'now',completedAt:'now',stale:false,generatorProfile:'g',embeddingProfile:'e',sources,findings:[{leftId:'left',rightId:'right',leftVersion:1,rightVersion:2,verdict:'collision',explanation:'Для одной оплаты указаны взаимоисключающие способы.'}],...overrides});
async function fixture(page:Page,initial:unknown=null,onStart?:()=>unknown){
 let latest=initial,calls=0;
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{const {query}=route.request().postDataJSON();let data:unknown;
  if(query.includes('startProjectAnalysis(')){calls++;latest=onStart?onStart():complete();data={startProjectAnalysis:latest}}
  else if(query.includes('projectAnalysis('))data={projectAnalysis:latest};
  else if(query.includes('requirementIndex('))data={requirementIndex:[]};
  else if(query.includes('requirements('))data={requirements:sources.map(source=>({...source,projectId:'demo',createdAt:'now',updatedAt:'now'}))};
  else data={project:{id:'demo',name:'Анализ проекта',goal:'G',description:'',createdAt:'now',updatedAt:'now'}};
  await route.fulfill({json:{data}});
 });return ()=>calls;
}
test('analysis runs only after explicit action and shows versioned sources',async({page})=>{
 const calls=await fixture(page);await page.goto('/projects/demo');await expect(page.getByRole('heading',{name:'Анализ возможных коллизий'})).toBeVisible();expect(calls()).toBe(0);
 await page.getByRole('button',{name:'Проверить требования',exact:true}).click();await expect(page.getByText('Возможная коллизия найдена',{exact:true})).toBeVisible();expect(calls()).toBe(1);
 await expect(page.getByText('Для одной оплаты указаны взаимоисключающие способы.',{exact:true})).toBeVisible();await expect(page.getByText('Отсутствие найденных коллизий не доказывает согласованность требований.')).toBeVisible();
 const analysis=page.getByRole('region',{name:'Анализ возможных коллизий'});await expect(analysis.getByText('left · Версия 1',{exact:true})).toBeVisible();await expect(analysis.getByText('right · Версия 2',{exact:true})).toBeVisible();
 await analysis.getByRole('link',{name:'Перейти к требованию'}).first().click();await expect(page).toHaveURL(/#requirement-left$/);
});
test('failed analysis can be retried and stale no-collision result is qualified',async({page})=>{
 let attempt=0;const calls=await fixture(page,null,()=>++attempt===1?complete({status:'failed',outcome:'insufficient',errorCode:'GENERATOR_FAILED',findings:[]}):complete({outcome:'no_collisions_found',stale:true,findings:[]}));
 await page.goto('/projects/demo');await page.getByRole('button',{name:'Проверить требования',exact:true}).click();await expect(page.getByText('Анализ не выполнен. Можно повторить попытку.',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'Повторить анализ',exact:true}).click();expect(calls()).toBe(2);await expect(page.getByText('В проверенных парах коллизий не найдено',{exact:true})).toBeVisible();
 await expect(page.getByText('Результат устарел: требования или профиль модели изменились. Повторите анализ.',{exact:true})).toBeVisible();await expect(page.getByText('Сохранённые версии источников на момент анализа',{exact:true})).toBeVisible();
});

test('a late metadata response cannot replace an explicitly started run',async({page})=>{
 let releaseMetadata!:()=>void,releaseStart!:()=>void;const metadata=new Promise<void>(resolve=>releaseMetadata=resolve);const start=new Promise<void>(resolve=>releaseStart=resolve);let first=true;
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{const {query}=route.request().postDataJSON();let data:unknown;
  if(query.includes('startProjectAnalysis(')){await start;data={startProjectAnalysis:complete()}}
  else if(query.includes('projectAnalysis(')){if(first){first=false;await metadata;data={projectAnalysis:null}}else data={projectAnalysis:complete()}}
  else if(query.includes('requirementIndex('))data={requirementIndex:[]};
  else if(query.includes('requirements('))data={requirements:[]};
  else data={project:{id:'demo',name:'Metadata race',goal:'G',description:'',createdAt:'now',updatedAt:'now'}};
  await route.fulfill({json:{data}});
 });
 await page.goto('/projects/demo');await page.getByRole('button',{name:'Проверить требования',exact:true}).click();await expect(page.getByRole('button',{name:'Выполняется анализ…',exact:true})).toBeDisabled();await expect(page.getByText('Анализ выполняется',{exact:true})).toBeVisible();
 releaseStart();await expect(page.getByText('Возможная коллизия найдена',{exact:true})).toBeVisible();releaseMetadata();await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));await expect(page.getByText('Возможная коллизия найдена',{exact:true})).toBeVisible();
});

test('persisted running analysis can be checked or retried explicitly',async({page})=>{
 const calls=await fixture(page,complete({status:'running',outcome:'insufficient',findings:[]}));await page.goto('/projects/demo');await expect(page.getByText('Анализ выполняется',{exact:true})).toBeVisible();expect(calls()).toBe(0);
 await page.getByRole('button',{name:'Проверить статус / повторить',exact:true}).click();expect(calls()).toBe(1);await expect(page.getByText('Возможная коллизия найдена',{exact:true})).toBeVisible();
});

test('invalidation refreshes a completed result to stale without rerunning the model',async({page})=>{
 let release!:()=>void;const event=new Promise<void>(resolve=>release=resolve);let current=complete();let calls=0;
 await page.route('**/api/events',async route=>{await event;await route.fulfill({status:200,contentType:'text/event-stream',body:'id: changed\nevent: cache.invalidate\ndata: {"schemaVersion":1,"keys":["project:demo"]}\n\n'})});
 await page.route('**/api/graphql',async route=>{const {query}=route.request().postDataJSON();let data:unknown;
  if(query.includes('startProjectAnalysis(')){calls++;data={startProjectAnalysis:current}}
  else if(query.includes('projectAnalysis('))data={projectAnalysis:current};
  else if(query.includes('requirementIndex('))data={requirementIndex:[]};
  else if(query.includes('requirements('))data={requirements:[]};
  else data={project:{id:'demo',name:'Source change',goal:'G',description:'',createdAt:'now',updatedAt:'now'}};
  await route.fulfill({json:{data}});
 });
 await page.goto('/projects/demo');await expect(page.getByText('Возможная коллизия найдена',{exact:true})).toBeVisible();current=complete({stale:true});release();await expect(page.getByText('Результат устарел: требования или профиль модели изменились. Повторите анализ.',{exact:true})).toBeVisible();expect(calls).toBe(0);
});

test('incomplete index has a specific explanation and source links',async({page})=>{
 await fixture(page,complete({outcome:'insufficient',reasonCode:'INDEX_INCOMPLETE',missingRequirementIds:['right'],findings:[]}));
 await page.goto('/projects/demo');
 const analysis=page.getByRole('region',{name:'Анализ возможных коллизий'});
 await expect(analysis.getByText('Не все требования проиндексированы. Проиндексируйте актуальные версии требований и повторите анализ.',{exact:true})).toBeVisible();
 const missing=analysis.getByRole('group',{name:'Требуют индексации: 1'});
 await missing.getByRole('link',{name:'right'}).click();await expect(page).toHaveURL(/#requirement-right$/);
});

test('model insufficient conditions is distinct from missing index',async({page})=>{
 await fixture(page,complete({outcome:'insufficient',reasonCode:'MODEL_INSUFFICIENT',missingRequirementIds:[],findings:[]}));
 await page.goto('/projects/demo');
 const analysis=page.getByRole('region',{name:'Анализ возможных коллизий'});
 await expect(analysis.getByText('В требованиях недостаточно условий для сравнения. Уточните их с учётом объяснения модели.',{exact:true})).toBeVisible();
 await expect(analysis.getByText('Не все требования проиндексированы.',{exact:false})).toHaveCount(0);
});
