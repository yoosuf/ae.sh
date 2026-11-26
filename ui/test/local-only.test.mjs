import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");

test("viewer source has no runtime network or write path", () => {
  const sources = [
    read("../src/main.tsx"),
    read("../src/parseSnapshot.ts"),
    read("../src/App.tsx"),
    read("../index.html"),
  ].join("\n");
  assert.doesNotMatch(sources, /\bfetch\s*\(/);
  assert.doesNotMatch(sources, /XMLHttpRequest/);
  assert.doesNotMatch(sources, /https?:\/\//);
  assert.doesNotMatch(sources, /localStorage|sessionStorage|indexedDB/);
});

test("viewer reads the embedded snapshot element", () => {
  const parser = read("../src/parseSnapshot.ts");
  assert.match(parser, /getElementById\("ae-snapshot"\)/);
  assert.match(read("../index.html"), /id="ae-snapshot"/);
});
