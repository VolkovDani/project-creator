import {test,expect} from '@playwright/test';
test('create a project and open it after reload',async({page})=>{
 const name=`Проект ${Date.now()}`;
 await page.goto('/projects/new');
 await page.getByRole('button',{name:'Создать проект'}).click();
 await expect(page.getByText('Заполните поле').first()).toBeVisible();
 await page.getByLabel('Название').fill(name);
 await page.getByLabel('Цель').fill('Найти коллизии');
 await page.getByLabel('Описание').fill('Тестовая карточка');
 await page.getByRole('button',{name:'Создать проект'}).click();
 await expect(page.getByRole('heading',{name,exact:true})).toBeVisible();
 await page.reload();await expect(page.getByRole('heading',{name,exact:true})).toBeVisible();await expect(page.getByText('Найти коллизии')).toBeVisible();await page.getByRole('link',{name:'Требования',exact:true}).first().click();
 await page.getByLabel('Текст требования').fill('Требование один');await page.getByRole('button',{name:'Добавить требование'}).click();await expect(page.getByText('Требование один',{exact:true})).toBeVisible();await page.reload();await expect(page.getByText('Требование один',{exact:true})).toBeVisible();
 const other=await page.context().newPage();await other.goto(page.url());await other.getByRole('button',{name:'Изменить'}).click();await other.getByLabel('Текст требования').fill('Мой черновик');
 await page.getByRole('button',{name:'Изменить'}).click();await page.getByLabel('Текст требования').fill('Требование два');await page.getByRole('button',{name:'Сохранить'}).click();await expect(page.getByText('Версия 2')).toBeVisible();
 await other.getByRole('button',{name:'Сохранить'}).click();await expect(other.getByText('Требование изменилось или недоступно. Обновите данные.')).toBeVisible();await expect(other.getByLabel('Текст требования')).toHaveValue('Мой черновик');await other.getByRole('button',{name:'Сохранить черновик поверх актуальной версии'}).click();await other.getByRole('button',{name:'Сохранить',exact:true}).click();await expect(other.getByText('Версия 3')).toBeVisible();
 await page.reload();await expect(page.getByText('Мой черновик',{exact:true})).toBeVisible();await page.getByRole('button',{name:'Удалить'}).click();await expect(page.getByText('Требований пока нет')).toBeVisible();await other.close();
});
test('create stays locked until its response completes',async({page})=>{
 let release!:()=>void;const pending=new Promise<void>(resolve=>{release=resolve});let calls=0;
 await page.route('**/api/events',route=>route.fulfill({status:200,contentType:'text/event-stream',body:': heartbeat\n\n'}));
 await page.route('**/api/graphql',async route=>{const payload=route.request().postDataJSON();if(payload.query.includes('mutation CreateProject')){calls++;await pending;await route.fulfill({json:{data:{createProject:{project:{id:'new',name:'new',goal:'goal',description:'',createdAt:'now',updatedAt:'now'},fieldErrors:[]}}}})} else {await route.fulfill({json:{data:{project:{id:'new',name:'new',goal:'goal',description:'',createdAt:'now',updatedAt:'now'}}}})}});
 await page.goto('/projects/new');await page.getByLabel('Название').fill('new');await page.getByLabel('Цель').fill('goal');await page.getByRole('button',{name:'Создать проект'}).click();await expect(page.getByRole('button',{name:'Создание…'})).toBeDisabled();expect(calls).toBe(1);release();await expect(page.getByRole('heading',{name:'new',exact:true})).toBeVisible();
});

test('two tabs receive project and requirement invalidations without reload',async({page})=>{
 const watcher=await page.context().newPage();await page.goto('/');await watcher.goto('/');
 const name=`Live ${Date.now()}`;await page.getByRole('link',{name:'Создать проект',exact:true}).click();await page.getByLabel('Название').fill(name);await page.getByLabel('Цель').fill('SSE');await page.getByRole('button',{name:'Создать проект'}).click();
 await expect(watcher.getByRole('link',{name})).toBeVisible({timeout:15000});await watcher.getByRole('link',{name}).click();await page.getByRole('link',{name:'Требования',exact:true}).first().click();await watcher.getByRole('link',{name:'Требования',exact:true}).first().click();
 await page.getByLabel('Текст требования').fill('Live requirement');await page.getByRole('button',{name:'Добавить требование'}).click();await expect(watcher.getByText('Live requirement',{exact:true})).toBeVisible({timeout:15000});
 await page.getByRole('button',{name:'Изменить'}).click();await page.getByLabel('Текст требования').fill('Changed in another tab');await page.getByRole('button',{name:'Сохранить',exact:true}).click();await expect(watcher.getByText('Changed in another tab',{exact:true})).toBeVisible({timeout:15000});
 await page.getByRole('button',{name:'Удалить'}).click();await expect(watcher.getByText('Требований пока нет')).toBeVisible({timeout:15000});await watcher.close();
});
