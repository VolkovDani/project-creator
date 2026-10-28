import {test} from 'node:test';
import assert from 'node:assert/strict';
import {indexStatus,requirementAnchor} from '../lib/rag.ts';
test('index from an old requirement version is shown as pending',()=>{assert.equal(indexStatus({version:1,status:'ready'},2),'pending');assert.equal(indexStatus({version:2,status:'ready'},2),'ready');assert.equal(indexStatus({version:2,status:'failed'},2),'failed');assert.equal(indexStatus(undefined,1),'pending')});
test('source fragment cannot introduce another URL',()=>{assert.equal(requirementAnchor('a/#? b'),'#requirement-a%2F%23%3F%20b')});
