import {test,expect} from '@playwright/test';
import {mkdir} from 'node:fs/promises';
import {resolve} from 'node:path';
test('screenshots: desktop projects and requirements',async({page},testInfo)=>{
 if(process.env.SCREENSHOT_FIXTURES==='1'){
  let project:Record<string,unknown>|null=null;const requirements:Record<string,unknown>[]=[];
  await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
  await page.route('**/api/graphql',async route=>{const {query,variables}=route.request().postDataJSON();let data:unknown;
   if(query.includes('createProject')){project={...variables.input,id:'demo-project',createdAt:'2026-10-27T12:00:00Z',updatedAt:'2026-10-27T12:00:00Z'};data={createProject:{project,fieldErrors:[]}}}
   else if(query.includes('createRequirement')){const requirement={...variables.input,id:`requirement-${requirements.length+1}`,version:1,createdAt:'2026-10-27T12:00:00Z',updatedAt:'2026-10-27T12:00:00Z'};requirements.push(requirement);data={createRequirement:{requirement,fieldErrors:[]}}}
   else if(query.includes('projectAnalysis('))data={projectAnalysis:null};
  else if(query.includes('requirementIndex('))data={requirementIndex:requirements.map(item=>({requirementId:item.id,version:item.version,status:'pending',profile:'',errorCode:''}))};
   else if(query.includes('requirements('))data={requirements};
   else if(query.includes('projects('))data={projects:project?[project]:[]};
   else data={project};
   await route.fulfill({json:{data}});
  });
 }
 await page.setViewportSize({width:1920,height:1080});await page.goto('/');
 await page.getByLabel('Название').fill('Контроль бизнес-требований');await page.getByLabel('Цель').fill('Согласовывать требования и находить возможные противоречия до начала разработки.');await page.getByLabel('Описание').fill('Единое пространство для проектов, требований и истории изменений.');await page.getByRole('button',{name:'Создать проект'}).click();
 const link=page.getByRole('link',{name:'Контроль бизнес-требований',exact:true});await expect(link).toBeVisible();
 const directory=resolve('screenshots');await mkdir(directory,{recursive:true});const projects=resolve(directory,'projects-desktop.png');await page.screenshot({path:projects,fullPage:false});await testInfo.attach('Projects — 1920×1080',{path:projects,contentType:'image/png'});
 await link.click();await expect(page.getByRole('heading',{name:'Контроль бизнес-требований',exact:true})).toBeVisible();
 for(const text of ['Менеджер может создавать проекты и описывать их цели.','Требования сохраняются между перезапусками приложения.','При одновременном редактировании система предотвращает потерю изменений.']){await page.getByLabel('Текст требования').fill(text);await page.getByRole('button',{name:'Добавить требование'}).click();await expect(page.getByText(text,{exact:true})).toBeVisible()}
 const requirements=resolve(directory,'project-requirements-desktop.png');await page.screenshot({path:requirements,fullPage:false});await testInfo.attach('Project requirements — 1920×1080',{path:requirements,contentType:'image/png'});
});
