// Tests for the JSON view of the row inspector. Run them with `npm test`.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { prettyJson } from '../src/lib/json'

test('an object from a json column is indented', () => {
  assert.equal(prettyJson('{"a":1,"b":[true,null]}', 'jsonb'), '{\n  "a": 1,\n  "b": [\n    true,\n    null\n  ]\n}')
})

test('an object or array in a text column is indented too', () => {
  assert.equal(prettyJson(' [1,2] ', 'text'), '[\n  1,\n  2\n]')
})

test('scalars, plain text, NULL and cut-off values stay as they are', () => {
  assert.equal(prettyJson('42', 'json'), null)
  assert.equal(prettyJson('"x"', 'json'), null)
  assert.equal(prettyJson('hello', 'text'), null)
  assert.equal(prettyJson('{not json', 'text'), null)
  assert.equal(prettyJson('{"a":"long…', 'json'), null)
  assert.equal(prettyJson(null, 'json'), null)
})
