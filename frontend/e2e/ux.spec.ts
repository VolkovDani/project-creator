import {test,expect,type Page} from '@playwright/test';

const project={id:'demo',name:'Стабильный проект',goal:'Проверить обновления',description:'Описание проекта',createdAt:'now',updatedAt:'now'};
const requirement=(id:number)=>({id:`r${id}`,projectId:'demo',text:`Требование ${id}`,version:1,createdAt:'now',updatedAt:'now'});

async function fixture(page:Page,count=21){
 const rows=Array.from({length:count},(_,i)=>requirement(i));
 const waiting=new Set<(body:string)=>void>();
 let delay:Promise<void>|undefined;
 let failure=0;
 let connections=0;
 let reads=0;
 let indexes=rows.map((r,i)=>({requirementId:r.id,version:1,status:i===0?'ready':i===1?'failed':'pending',profile:'fixture',errorCode:''}));
 await page.route('**/api/events',async route=>{
  connections++;
  const body=await new Promise<string>(resolve=>waiting.add(resolve));
  await route.fulfill({status:200,contentType:'text/event-stream',body}).catch(()=>{});
 });
 await page.route('**/api/graphql',async route=>{
  const {query,variables}=route.request().postDataJSON();
  let data:unknown;
  if(query.includes('requirements(')){
   reads++;if(delay)await delay;
   if(failure){await route.fulfill({status:failure,json:{error:'fixture failure'}});return}
   data={requirements:rows.slice(variables.offset,variables.offset+variables.limit)};
  }else if(query.includes('requirementIndex('))data={requirementIndex:indexes.slice(variables.offset,variables.offset+variables.limit)};
  else if(query.includes('searchRequirements('))data={searchRequirements:[{requirementId:'r20',projectId:'demo',text:'Результат поиска',sourceText:rows[20]?.text||'Требование 20',version:1,start:0,end:12,score:1}]};
  else if(query.includes('analysisHistory('))data={analysisHistory:[]};
  else if(query.includes('projectAnalysis('))data={projectAnalysis:null};
  else if(query.includes('projects('))data={projects:[project]};
  else data={project};
  await route.fulfill({json:{data}}).catch(()=>{});
 });
 return {
  reads:()=>reads,
  connections:()=>connections,
  fail:(status:number)=>{failure=status},
  delay:()=>{let release!:()=>void;delay=new Promise<void>(resolve=>release=resolve);return ()=>{delay=undefined;release()}},
  setText:(text:string)=>{rows[0]={...rows[0],text}},
  setReady:()=>{indexes=indexes.map(x=>({...x,status:'ready'}))},
  async event(reset=false){
   await expect.poll(()=>waiting.size).toBeGreaterThan(0);
   const body=`id: ${Date.now()}\nevent: ${reset?'cache.reset':'cache.invalidate'}\ndata: ${JSON.stringify({schemaVersion:1,keys:['projects','project:demo','project:demo:requirements']})}\n\n`;
   for(const resolve of waiting)resolve(body);waiting.clear();
  },
 };
}

test('first load reserves space and a failed first request has a local retry',async({page})=>{
 const api=await fixture(page);const release=api.delay();await page.goto('/projects/demo/requirements');await expect.poll(api.reads).toBeGreaterThan(0);
 await expect(page.getByText('Загрузка…',{exact:true})).toBeVisible();
 await expect(page.locator('.skeleton')).toBeVisible();await expect(page.getByText('Требований пока нет',{exact:true})).toHaveCount(0);
 api.fail(503);release();await expect(page.getByRole('button',{name:'Повторить',exact:true})).toBeVisible();
 await expect(page.locator('#requirement-r0')).toHaveCount(0);
 api.fail(0);await page.getByRole('button',{name:'Повторить',exact:true}).click();await expect(page.locator('#requirement-r0')).toBeVisible();
});

test('slow SSE refetch and reconnect reset preserve cards, draft, focus and pagination',async({page})=>{
 const api=await fixture(page);
 await page.goto('/projects/demo/requirements');
 const card=page.locator('#requirement-r0');
 await expect(card).toBeVisible();
 const draft=page.getByLabel('Текст требования');await draft.fill('Несохранённый черновик');await draft.focus();
 const before=await card.boundingBox();
 const reads=api.reads();const connections=api.connections();const release=api.delay();await api.event();
 await expect.poll(api.reads).toBeGreaterThan(reads);
 await expect(card).toBeVisible();await expect(draft).toHaveValue('Несохранённый черновик');await expect(draft).toBeFocused();
 expect((await card.boundingBox())?.y).toBe(before?.y);
 api.setText('Обновлённое требование');release();await expect(card.getByText('Обновлённое требование',{exact:true})).toBeVisible();
 await expect.poll(api.connections).toBeGreaterThan(connections);
 const resetReads=api.reads();const releaseReset=api.delay();await api.event(true);await expect.poll(api.reads).toBeGreaterThan(resetReads);
 await expect(card).toBeVisible();await expect(draft).toHaveValue('Несохранённый черновик');releaseReset();
 await page.getByRole('button',{name:'Далее',exact:true}).click();await expect(page.locator('#requirement-r20')).toBeVisible();
 await expect(page.getByText('Страница 2',{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'Назад',exact:true}).click();await expect(card).toBeVisible();
});

test('failed background request keeps stale data and local retry restores freshness',async({page})=>{
 const api=await fixture(page);await page.goto('/projects/demo/requirements');await expect(page.locator('#requirement-r0')).toBeVisible();
 api.fail(503);await api.event();
 await expect(page.getByText('Показаны последние загруженные данные. Они могут быть устаревшими.',{exact:false})).toBeVisible();
 await expect(page.locator('#requirement-r0')).toBeVisible();
 api.fail(0);api.setText('Успешный повтор');await page.getByRole('button',{name:'Повторить',exact:true}).click();
 await expect(page.getByText('Успешный повтор',{exact:true})).toBeVisible();
 await expect(page.getByText('Показаны последние загруженные данные. Они могут быть устаревшими.',{exact:false})).toHaveCount(0);
});

test('revoked access clears previously rendered data',async({page})=>{
 const api=await fixture(page);await page.goto('/projects/demo/requirements');await expect(page.locator('#requirement-r0')).toBeVisible();
 api.fail(403);await api.event();await expect(page.locator('#requirement-r0')).toHaveCount(0);
 await expect(page.getByText('Требование 0',{exact:true})).toHaveCount(0);
});

test('project pages share navigation, locale and browser history without overflow',async({page})=>{
 await fixture(page);await page.goto('/');
 await expect(page.getByRole('link',{name:project.name,exact:true})).toBeVisible();
 await expect(page.getByLabel('Название',{exact:true})).toHaveCount(0);
 await page.getByRole('link',{name:'Создать проект',exact:true}).click();await expect(page).toHaveURL(/\/projects\/new$/);await expect(page.getByLabel('Название',{exact:true})).toBeVisible();
 await page.goto('/projects/demo');await expect(page.getByRole('heading',{name:project.name,exact:true})).toBeVisible();
 await expect(page.getByLabel('Текст требования')).toHaveCount(0);
 await page.getByLabel('Язык').selectOption('en');
 for(const [name,path] of [['Requirements','requirements'],['Indexing','indexing'],['Search','search'],['Analysis','analysis']] as const){
  await page.getByRole('link',{name,exact:true}).first().click();await expect(page).toHaveURL(new RegExp(`/${path}$`));
  await expect(page.getByRole('link',{name,exact:true}).first()).toHaveAttribute('aria-current','page');
  await expect(page.getByLabel('Language')).toHaveValue('en');
 }
 await page.goBack();await expect(page).toHaveURL(/\/search$/);await page.goForward();await expect(page).toHaveURL(/\/analysis$/);
 await page.reload();await expect(page.getByLabel('Language')).toHaveValue('en');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
});

test('requirement source links select the correct paginated card and survive direct opening',async({page})=>{
 await fixture(page);await page.goto('/projects/demo/search');
 await page.getByLabel('Поисковый запрос').fill('20');await page.getByRole('button',{name:'Найти требования',exact:true}).click();
 await page.getByRole('link',{name:'Перейти к требованию',exact:true}).click();
 await expect(page).toHaveURL(/\/requirements\?requirement=r20#requirement-r20$/);
 await expect(page.locator('#requirement-r20')).toBeVisible();await expect(page.getByText('Страница 2',{exact:true})).toBeVisible();
 await page.reload();await expect(page.locator('#requirement-r20')).toBeVisible();
 await page.goBack();await expect(page).toHaveURL(/\/search$/);
});

test('index group summaries are scoped to current page and update while collapsed',async({page})=>{
 const api=await fixture(page);await page.goto('/projects/demo/indexing');
 await expect(page.getByText(/На этой странице/).first()).toBeVisible();
 const summaries=page.locator('details summary');await expect(summaries.first()).toBeVisible();
 const initial=await summaries.allTextContents();expect(initial.join(' ')).toMatch(/20/);
 await summaries.first().focus();await page.keyboard.press('Enter');
 await expect(page.locator('details').first()).not.toHaveAttribute('open','');
 api.setReady();await api.event();
 await expect.poll(async()=>(await summaries.allTextContents()).join(' ')).not.toBe(initial.join(' '));
 await expect(page.locator('details').first()).not.toHaveAttribute('open','');
 await page.getByRole('button',{name:'Далее',exact:true}).click();await expect(page.getByText('Страница 2',{exact:true})).toBeVisible();
 await expect.poll(async()=>(await summaries.allTextContents()).join(' ')).not.toMatch(/20/);
});

test('a failed project page transition keeps its rows and page number coherent until retry',async({page})=>{
 let failed=true;
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{
  const {variables}=route.request().postDataJSON();
  if(variables.offset===20&&failed){await route.fulfill({status:503,json:{error:'unavailable'}});return}
  const projects=Array.from({length:variables.offset===20?1:20},(_,i)=>({...project,id:`p${variables.offset+i}`,name:`Проект ${variables.offset+i}`}));
  await route.fulfill({json:{data:{projects}}});
 });
 await page.goto('/');await expect(page.getByRole('link',{name:'Проект 0',exact:true})).toBeVisible();
 await page.getByRole('button',{name:'Далее',exact:true}).click();
 await expect(page.getByRole('button',{name:'Повторить',exact:true})).toBeVisible();
 await expect(page.getByRole('link',{name:'Проект 0',exact:true})).toBeVisible();
 await expect(page.getByText('Страница 1',{exact:true})).toBeVisible();
 failed=false;await page.getByRole('button',{name:'Повторить',exact:true}).click();
 await expect(page.getByRole('link',{name:'Проект 20',exact:true})).toBeVisible();
 await expect(page.getByText('Страница 2',{exact:true})).toBeVisible();
});

test('a late create response does not redirect after the user has left the form',async({page})=>{
 let release!:()=>void,started!:()=>void;
 const pending=new Promise<void>(resolve=>release=resolve),sent=new Promise<void>(resolve=>started=resolve);
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{
  const {query}=route.request().postDataJSON();
  if(query.includes('createProject(')){started();await pending;await route.fulfill({json:{data:{createProject:{project:{...project,id:'created'},fieldErrors:[]}}}});return}
  await route.fulfill({json:{data:query.includes('projects(')?{projects:[project]}:{project}}});
 });
 await page.goto('/projects/new');await page.getByLabel('Название').fill('Новый проект');await page.getByLabel('Цель').fill('Цель');
 await page.getByRole('button',{name:'Создать проект',exact:true}).click();await sent;
 await page.getByRole('link',{name:'Все проекты',exact:true}).click();await expect(page).toHaveURL(/\/$/);
 const response=page.waitForResponse(response=>response.url().endsWith('/api/graphql')&&response.request().postDataJSON().query.includes('createProject('));
 release();await (await response).finished();await page.evaluate(()=>new Promise<void>(resolve=>requestAnimationFrame(()=>requestAnimationFrame(()=>resolve()))));
 await expect(page).toHaveURL(/\/$/);await expect(page.getByRole('link',{name:project.name,exact:true})).toBeVisible();
});
