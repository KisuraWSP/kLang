let klangGoRuntime = null;
let klangWasmStarted = false;
let klangProject = null;

async function startKlangWASM() {
  if (klangWasmStarted) return;
  klangGoRuntime = new Go();
  const response = await fetch("klang.wasm");
  const bytes = await response.arrayBuffer();
  const result = await WebAssembly.instantiate(bytes, klangGoRuntime.importObject);
  klangWasmStarted = true;
  klangGoRuntime.run(result.instance);
  await Promise.resolve();
}

async function loadKlangProject() {
  if (klangProject) return klangProject;
  const manifest = await fetch("klang-build.json").then((response) => response.json());
  const files = {};
  for (const file of manifest.files) {
    files[file] = await fetch(file).then((response) => response.text());
  }
  klangProject = {
    name: manifest.project_name,
    entry: manifest.entry,
    files,
  };
  return klangProject;
}

async function runKlangProject(args = []) {
  await startKlangWASM();
  const project = await loadKlangProject();
  return JSON.parse(globalThis.klangRunProject(project, args));
}

async function checkKlangProject() {
  await startKlangWASM();
  const project = await loadKlangProject();
  return JSON.parse(globalThis.klangCheckProject(project));
}

async function runKlangSource(source, args = []) {
  await startKlangWASM();
  return JSON.parse(globalThis.klangRun(source, args));
}

globalThis.KlangBrowser = {
  start: startKlangWASM,
  loadProject: loadKlangProject,
  runProject: runKlangProject,
  checkProject: checkKlangProject,
  runSource: runKlangSource,
};
