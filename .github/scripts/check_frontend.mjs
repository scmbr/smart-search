import { readFileSync, existsSync } from "node:fs";

// A new frontend must supply real, non-watch CI scripts before it can be merged.
for (const file of ["frontend/package.json", "frontend/package-lock.json", "frontend/.nvmrc"]) {
  if (!existsSync(file)) throw new Error(`Missing required frontend configuration: ${file}`);
}
const pkg = JSON.parse(readFileSync("frontend/package.json", "utf8"));
for (const name of ["lint", "lint:arch", "typecheck", "test:ci", "build"]) {
  if (typeof pkg.scripts?.[name] !== "string" || !pkg.scripts[name].trim()) {
    throw new Error(`Missing frontend script: ${name}`);
  }
}
console.log("PASS: frontend declares required CI scripts");
