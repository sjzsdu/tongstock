import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dirname, '../..');
const contractPath = resolve(root, 'api/openapi.json');
const outputPath = resolve(root, 'web/src/api/generated.ts');
const contract = JSON.parse(readFileSync(contractPath, 'utf8'));

function typeOf(schema = {}) {
  if (schema.$ref) return schema.$ref.split('/').at(-1);
  if (schema.const !== undefined) return JSON.stringify(schema.const);
  if (schema.type === 'array') return `${typeOf(schema.items)}[]`;
  if (schema.type === 'string') return 'string';
  if (schema.type === 'number' || schema.type === 'integer') return 'number';
  if (schema.type === 'boolean') return 'boolean';
  if (schema.type === 'object') return 'Record<string, unknown>';
  return 'unknown';
}

const lines = [
  '// Generated from api/openapi.json. Do not edit by hand.',
  '',
];
for (const [name, schema] of Object.entries(contract.components.schemas)) {
  const required = new Set(schema.required || []);
  if (schema.type !== 'object') {
    lines.push(`export type ${name} = ${typeOf(schema)};`, '');
    continue;
  }
  lines.push(`export interface ${name} {`);
  for (const [property, propertySchema] of Object.entries(schema.properties || {})) {
    lines.push(`  ${JSON.stringify(property)}${required.has(property) ? '' : '?'}: ${typeOf(propertySchema)};`);
  }
  lines.push('}', '');
}

lines.push('export const operations = {');
for (const [path, pathItem] of Object.entries(contract.paths)) {
  for (const [method, operation] of Object.entries(pathItem)) {
    lines.push(`  ${operation.operationId}: { method: ${JSON.stringify(method.toUpperCase())}, path: ${JSON.stringify(path)} },`);
  }
}
lines.push('} as const;', '');

const generated = lines.join('\n');
if (process.argv.includes('--check')) {
  const current = readFileSync(outputPath, 'utf8');
  if (current !== generated) {
    console.error('Generated API client is stale. Run: pnpm api:generate');
    process.exit(1);
  }
} else {
  writeFileSync(outputPath, generated);
}
