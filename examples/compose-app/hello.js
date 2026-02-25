// Prints the remoteEnv variables injected by crib.
// Run with: crib exec -- node hello.js
const project = process.env.PROJECT_NAME || '(not set)';
const redis = process.env.REDIS_URL || '(not set)';
const nodeEnv = process.env.NODE_ENV || '(not set)';

console.log(`Project:  ${project}`);
console.log(`Redis:    ${redis}`);
console.log(`NODE_ENV: ${nodeEnv}`);
