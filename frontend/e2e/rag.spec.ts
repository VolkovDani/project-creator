import {test,expect} from '@playwright/test';
test('index retry and search are explicit and retain versioned sources',async({page})=>{
 let indexCalls=0,searchCalls=0;let status='pending';const text='Пользователь может сохранить черновик требования.';
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{const {query}=route.request().postDataJSON();let data:unknown;
  if(query.includes('indexRequirement(')){indexCalls++;status='failed';data={indexRequirement:{requirementId:'source-1',version:2,status,profile:'',errorCode:'NOT_CONFIGURED'}}}
  else if(query.includes('requirementIndex('))data={requirementIndex:[{requirementId:'source-1',version:2,status,profile:'',errorCode:status==='failed'?'NOT_CONFIGURED':''}]};
  else if(query.includes('searchRequirements(')){searchCalls++;data={searchRequirements:[{requirementId:'source-1',projectId:'demo',text:'сохранить черновик',sourceText:text,version:2,start:23,end:41,score:0.91}]}}
  else if(query.includes('requirements('))data={requirements:[{id:'source-1',projectId:'demo',text,version:2,createdAt:'now',updatedAt:'now'}]};
  else data={project:{id:'demo',name:'Проект поиска',goal:'Проверить поиск',description:'',createdAt:'now',updatedAt:'now'}};
  await route.fulfill({json:{data}});
 });
 await page.goto('/projects/demo');await expect(page.getByText('Индекс: ожидает индексации')).toBeVisible();expect(indexCalls).toBe(0);expect(searchCalls).toBe(0);
 await page.getByRole('button',{name:'Индексировать',exact:true}).click();await expect(page.getByText('Локальная модель не настроена. Настройте embedding-провайдер по README.')).toBeVisible();expect(indexCalls).toBe(1);
 await page.getByRole('button',{name:'Повторить индексацию',exact:true}).click();await expect.poll(()=>indexCalls).toBe(2);
 await page.getByLabel('Поисковый запрос').fill('черновик');await page.getByRole('button',{name:'Найти требования'}).click();await expect(page.getByText('сохранить черновик',{exact:true})).toBeVisible();expect(searchCalls).toBe(1);
 await page.getByText('Исходный текст этой версии',{exact:true}).click();await expect(page.getByText(text,{exact:true})).toHaveCount(2);await page.getByRole('link',{name:'Перейти к требованию'}).click();await expect(page).toHaveURL(/#requirement-source-1$/);
});
test('search source opens the requirement on another list page',async({page})=>{
 const requirements=Array.from({length:21},(_,index)=>({id:`source-${index}`,projectId:'demo',text:`Требование ${index}`,version:1,createdAt:'now',updatedAt:'now'}));
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{const {query,variables}=route.request().postDataJSON();let data:unknown;
  if(query.includes('requirementIndex('))data={requirementIndex:[]};
  else if(query.includes('searchRequirements('))data={searchRequirements:[{requirementId:'source-20',projectId:'demo',text:'Требование 20',sourceText:'Требование 20',version:1,start:0,end:13,score:1}]};
  else if(query.includes('requirements('))data={requirements:requirements.slice(variables.offset,variables.offset+variables.limit)};
  else data={project:{id:'demo',name:'Пагинация источников',goal:'G',description:'',createdAt:'now',updatedAt:'now'}};
  await route.fulfill({json:{data}});
 });
 await page.goto('/projects/demo');await expect(page.locator('#requirement-source-0')).toBeVisible();await expect(page.locator('#requirement-source-20')).toHaveCount(0);
 await page.getByLabel('Поисковый запрос').fill('20');await page.getByRole('button',{name:'Найти требования'}).click();await page.getByRole('link',{name:'Перейти к требованию'}).click();await expect(page.locator('#requirement-source-20')).toBeVisible();await expect(page).toHaveURL(/#requirement-source-20$/);
});
