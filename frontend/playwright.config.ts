import {defineConfig} from '@playwright/test';
if (!process.env.E2E_DATABASE_URL) throw new Error('E2E_DATABASE_URL must point to a disposable PostgreSQL database');
const token='local-test-token-0123456789-0123456789';
export default defineConfig({testDir:'./e2e',timeout:30000,fullyParallel:false,workers:1,use:{baseURL:'http://127.0.0.1:3100',launchOptions:process.env.E2E_CHROMIUM_PATH?{executablePath:process.env.E2E_CHROMIUM_PATH,args:['--no-sandbox']}:{}} ,webServer:[{command:'go run ../cmd/server',url:'http://127.0.0.1:8080/graphql',env:{PROJECT_CREATOR_TOKEN:token,PROJECT_CREATOR_DATABASE_URL:process.env.E2E_DATABASE_URL},reuseExistingServer:false,timeout:120000},{command:'npm run start -- -p 3100',url:'http://127.0.0.1:3100',env:{PROJECT_CREATOR_TOKEN:token},reuseExistingServer:false,timeout:120000}]});

