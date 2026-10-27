import {test} from 'node:test';
import {readFileSync} from 'node:fs';
import assert from 'node:assert/strict';
import {validate, validateContract} from '../lib/validation.ts';
const contract=JSON.parse(readFileSync('../contracts/project.json','utf8'));
const fixtures=JSON.parse(readFileSync('../contracts/project.fixtures.json','utf8'));
test('shared validation fixtures',()=>{for(const c of fixtures){const got=validate(contract,c.input);assert.deepEqual(got,{normalized:c.normalized,errors:c.errors},c.name)}});
test('unknown contract rules rejected',()=>{assert.throws(()=>validateContract({...contract,extra:true}));assert.throws(()=>validateContract({...contract,trim:'unicode'}))});
test('invalid UTF-16 rejected',()=>{const r=validate(contract,{name:'\ud800',goal:'x',description:''});assert.equal(r.errors[0].code,'INVALID_UTF8')});
