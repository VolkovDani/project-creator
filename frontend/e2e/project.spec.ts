import {test,expect} from '@playwright/test';
test('create a project and open it after reload',async({page})=>{
 const name=`Проект ${Date.now()}`;
 await page.goto('/');
 await page.getByRole('button',{name:'Создать проект'}).click();
 await expect(page.getByText('Заполните поле').first()).toBeVisible();
 await page.getByLabel('Название').fill(name);
 await page.getByLabel('Цель').fill('Найти коллизии');
 await page.getByLabel('Описание').fill('Тестовая карточка');
 await page.getByRole('button',{name:'Создать проект'}).click();
 await expect(page.getByRole('link',{name})).toBeVisible();
 await page.reload();await expect(page.getByRole('link',{name})).toBeVisible();
 await page.getByRole('link',{name}).click();await expect(page.getByRole('heading',{name})).toBeVisible();await expect(page.getByText('Найти коллизии')).toBeVisible();
});
test('pagination remains locked until create completes',async({page})=>{
 let release!:()=>void;const pending=new Promise<void>(resolve=>{release=resolve});
 await page.route('**/api/graphql',async route=>{const payload=route.request().postDataJSON();if(payload.query.includes('mutation CreateProject')){await pending;await route.fulfill({json:{data:{createProject:{project:{id:'new',name:'new',goal:'goal',description:'',createdAt:'now',updatedAt:'now'},fieldErrors:[]}}}})} else {const projects=Array.from({length:20},(_,i)=>({id:String(i),name:`Item ${i}`,goal:'goal',description:'',createdAt:'now',updatedAt:'now'}));await route.fulfill({json:{data:{projects}}})}});
 await page.goto('/');await expect(page.getByRole('link',{name:'Item 0'})).toBeVisible();await page.getByLabel('Название').fill('new');await page.getByLabel('Цель').fill('goal');await page.getByRole('button',{name:'Создать проект'}).click();await expect(page.getByRole('button',{name:'Далее'})).toBeDisabled();release();await expect(page.getByRole('button',{name:'Далее'})).toBeEnabled();
});
