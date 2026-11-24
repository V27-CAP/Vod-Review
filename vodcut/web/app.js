"use strict";
const $ = (id) => document.getElementById(id);
const dict = {
  local: ["Processamento local", "Local processing"],
  workspace: ["Workspace de edição", "Editing workspace"],
  desk: ["Mesa de cortes", "Review desk"],
  import: ["Carregar VOD", "Load VOD"],
  export: ["Exportar cortes", "Export clips"],
  monitor: ["Monitor de prévia", "Preview monitor"],
  queue: ["Fila de highlights", "Highlight queue"],
  signal: ["Intensidade dos sinais", "Signal intensity"],
  emptyMonitor: ["Nenhuma VOD carregada", "No VOD loaded"],
  emptyMonitorSub: [
    "Carregue uma gravação para encontrar os melhores trechos.",
    "Load a recording to find its highlights.",
  ],
  inspector: ["Inspetor de corte", "Clip inspector"],
  source: [
    "Arquivo de vídeo · MP4, MKV ou MOV",
    "Video file · MP4, MKV or MOV",
  ],
  chatfile: ["Log do chat (opcional)", "Chat log (optional)"],
  browse: ["Procurar", "Browse"],
  modules: ["Sinais de detecção", "Detection signals"],
  audio: ["Áudio", "Audio"],
  transcription: ["Transcrição local", "Local transcription"],
  visual: ["Mudanças visuais", "Visual changes"],
  judge: ["Avaliação por LLM local", "Local LLM judge"],
  limit: ["Máx. de cortes", "Max. clips"],
  min: ["Mínimo (s)", "Minimum (s)"],
  max: ["Máximo (s)", "Maximum (s)"],
  analyze: ["Analisar VOD", "Analyze VOD"],
  format: ["Formatos", "Formats"],
  sequence: ["Resolução da sequência XML", "XML sequence resolution"],
  match: ["Igual à fonte", "Match source"],
  all: ["Todos", "All"],
  approved: ["Aprovados", "Approved"],
  rejected: ["Rejeitados", "Rejected"],
  pending: ["Pendente", "Pending"],
  emptyQueue: ["Nenhum highlight ainda", "No highlights yet"],
  emptyQueueSub: [
    "Os trechos sugeridos aparecem aqui após a análise.",
    "Suggested moments will appear here after analysis.",
  ],
  emptyFilter: ["Nenhum corte neste filtro", "No clips in this filter"],
  noHits: ["Nenhum pico atingiu o limite", "No peaks reached the threshold"],
  noHitsSub: [
    "Ajuste os sinais ou o limite no config.toml e analise novamente.",
    "Adjust the signals or threshold in config.toml and analyze again.",
  ],
  choose: [
    "Selecione um highlight para revisar seus limites, sinais e transcrição.",
    "Select a highlight to review its timing, signals and transcript.",
  ],
  none: ["Nenhuma fonte selecionada", "No source selected"],
  selectClip: ["Selecione um corte", "Select a clip"],
  name: ["Nome do corte", "Clip name"],
  start: ["Início (s)", "Start (s)"],
  end: ["Fim (s)", "End (s)"],
  save: ["Salvar ajustes", "Save changes"],
  approve: ["Aprovar", "Approve"],
  reject: ["Rejeitar", "Reject"],
  peak: ["Pico", "Peak"],
  transcript: ["Transcrição", "Transcript"],
  noWords: [
    "Sem fala transcrita neste intervalo.",
    "No transcribed speech in this range.",
  ],
  cancel: ["Interromper", "Stop"],
  open: ["Abrir pasta", "Open folder"],
  saved: ["Ajustes salvos.", "Changes saved."],
  done: ["Exportação concluída", "Export complete"],
  probe: ["Leitura", "Source"],
  extract: ["Extração", "Extraction"],
  transcribe: ["Transcrição", "Transcription"],
  rank: ["Ranking", "Ranking"],
  chat: ["Chat", "Chat"],
  preparing: ["Preparando prévia…", "Preparing preview…"],
  previewError: [
    "A prévia falhou. Confira o arquivo de origem e o FFmpeg.",
    "Preview failed. Check the source file and FFmpeg.",
  ],
  durationError: [
    "O máximo precisa ser igual ou maior que o mínimo.",
    "Maximum must be at least the minimum.",
  ],
  formatError: ["Selecione um formato.", "Select a format."],
  score: ["Pontuação", "Score"],
  loudness: ["Volume", "Loudness"],
  change: ["Energia", "Energy"],
  vocal: ["Vocal", "Vocal"],
  chat_rate: ["Ritmo do chat", "Chat rate"],
  chat_laugh: ["Risadas", "Chat laughter"],
  chat_users: ["Participantes", "Chatters"],
  motion: ["Movimento", "Motion"],
  ctx: ["Há fala antes e depois do pico.", "Speech surrounds the peak."],
  loading: ["Iniciando…", "Starting…"],
  render: ["Renderização", "Rendering"],
};
Object.assign(dict, {
  hero1: ["Sua live em", "Turn your stream into"],
  hero2: ["bons cortes.", "great clips."],
  intro: [
    "Abra uma gravação para encontrar reações, piadas e surpresas que merecem virar um corte.",
    "Open a recording to find reactions, jokes and surprises worth clipping.",
  ],
  startHere: ["Selecionar gravação", "Choose a recording"],
  workflow: ["Como funciona", "How it works"],
  loadStep: ["Carregue sua live", "Load your stream"],
  reviewStep: ["Escolha os momentos", "Pick your moments"],
  exportStep: ["Finalize no Premiere", "Finish in Premiere"],
  loadStepHelp: [
    "Escolha um vídeo do seu computador. Se tiver o chat da live, adicione-o para ajudar na busca.",
    "Choose a video on your computer. Add the stream chat if you have it to help find reactions.",
  ],
  reviewStepHelp: [
    "Assista às sugestões, ajuste onde cada corte começa e termina e aprove seus favoritos.",
    "Preview the suggestions, trim their start and end points and approve your favorites.",
  ],
  exportStepHelp: [
    "Exporte os aprovados como uma sequência para editar ou salve cada trecho em MP4.",
    "Export approved clips as an editing sequence or save each one as an MP4.",
  ],
  setupIntro: [
    "Escolha a gravação original. O áudio e a fala serão analisados no seu computador com as opções padrão.",
    "Choose your original recording. The default settings analyze audio and speech on your computer.",
  ],
  advanced: ["Ajustar a detecção", "Customize detection"],
  advancedHint: [
    "Escolha quais pistas procurar e a duração dos cortes sugeridos.",
    "Choose which signals to look for and how long suggested clips should be.",
  ],
  audioHelp: [
    "Procura picos de volume e reações vocais.",
    "Looks for volume changes and vocal reactions.",
  ],
  transcriptionHelp: [
    "Usa as falas para dar contexto ao corte.",
    "Uses dialogue to find context for a clip.",
  ],
  visualHelp: [
    "Procura mudanças de cena. Pode demorar mais.",
    "Looks for scene changes. May take longer.",
  ],
  judgeHelp: [
    "Avalia o humor da fala. Requer um servidor de IA local configurado.",
    "Rates humor in dialogue. Requires a configured local AI server.",
  ],
  queueHint: [
    "Selecione um trecho para assistir. A pontuação indica a força das pistas encontradas.",
    "Select a clip to watch. Its score reflects the strength of the detected signals.",
  ],
  awaitApproval: [
    "Aprove um corte para liberar a exportação.",
    "Approve a clip to enable export.",
  ],
  exportReady: ["cortes prontos para exportar", "clips ready to export"],
  readyOne: ["1 corte pronto para exportar", "1 clip ready to export"],
  editHint: [
    "Os tempos são medidos desde o início da live. Mantenha o pico dentro do corte.",
    "Times are measured from the start of the stream. Keep the peak inside the clip.",
  ],
  fmtxml: ["Sequência do Premiere", "Premiere sequence"],
  fmtmp4: ["Vídeos MP4", "MP4 videos"],
  fmtcsv: ["Planilha CSV", "CSV spreadsheet"],
  fmtjson: ["Dados JSON", "JSON data"],
  helpxml: [
    "Importe o XML no Premiere para continuar a edição.",
    "Import the XML in Premiere to continue editing.",
  ],
  helpmp4: [
    "Um arquivo de vídeo para cada corte aprovado.",
    "One video file for each approved clip.",
  ],
  helpcsv: [
    "Nomes, tempos e notas em uma tabela.",
    "Clip names, timings and scores in a table.",
  ],
  helpjson: [
    "Lista de cortes para scripts e outras ferramentas.",
    "Clip list for scripts and other tools.",
  ],
  signal: ["Atividade ao longo da live", "Activity across the stream"],
  monitor: ["Prévia do corte", "Clip preview"],
  inspector: ["Ajustar corte", "Trim clip"],
  queue: ["Cortes sugeridos", "Suggested clips"],
  local: ["Estúdio local", "Local studio"],
  approve: ["Aprovar corte", "Approve clip"],
  reject: ["Descartar", "Reject"],
  evidenceTitle: ["Por que este trecho?", "Why this clip?"],
  judge: ["Análise por IA local", "Local AI review"],
  step1: ["ETAPA 01", "STEP 01"],
  step2: ["ETAPA 02", "STEP 02"],
  step3: ["ETAPA 03", "STEP 03"],
});
Object.assign(dict, {
  analysisTitle: ["Analisando sua gravação", "Analyzing your recording"],
  packingTitle: ["Preparando os cortes", "Preparing your clips"],
  waitHint: [
    "Mantenha o estúdio aberto. Acompanhe o progresso de cada etapa abaixo.",
    "Keep the studio open. Follow each stage below.",
  ],
  rejected: ["Descartados", "Rejected"],
});
Object.assign(dict, {
  appearance: ["Tema", "Theme"],
  red: ["Vermelho", "Red"],
  dark: ["Escuro", "Dark"],
  light: ["Claro", "Light"],
});
const themeOptions = [...document.querySelectorAll("#themes input")];
function setTheme(value) {
  const chosen =
    themeOptions.find((option) => option.value === value) || themeOptions[0];
  chosen.checked = true;
  document.documentElement.dataset.theme = chosen.value;
  const color = getComputedStyle(document.documentElement)
    .getPropertyValue("--mast")
    .trim();
  document.querySelector('meta[name="theme-color"]').content = color;
}
setTheme(localStorage.getItem("vodcut-theme"));
$("themes").onchange = (event) => {
  setTheme(event.target.value);
  localStorage.setItem("vodcut-theme", event.target.value);
  waveform();
};
let lang = localStorage.getItem("vodcut-lang") === "en" ? "en" : "pt";
const t = (k) => dict[k]?.[lang === "en" ? 1 : 0] ?? k;
const esc = (s) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const icons = {
  folder: "M3 6h6l2 2h10v11H3z M3 6V4h6l2 2",
  cut: "M5 5a2.5 2.5 0 1 0 .1 0 M5 15a2.5 2.5 0 1 0 .1 0 M8 8l12 11 M8 16L20 5",
  out: "M12 14V3m-4 4 4-4 4 4M5 13v7h14v-7",
  plus: "M12 5v14M5 12h14",
  monitor: "M3 4h18v13H3zM8 21h8m-4-4v4",
  stack: "m12 3 9 5-9 5-9-5Zm-9 9 9 5 9-5m-18 4 9 5 9-5",
  sliders: "M4 7h4m4 0h8M4 17h10m4 0h2M8 4h4v6H8zM14 14h4v6h-4z",
};
const icon = (k) =>
  '<svg aria-hidden="true" class="icon" viewBox="0 0 24 24"><path d="' +
  icons[k] +
  '"/></svg>';
let state = { project: null, job: {}, defaults: {} },
  selected = null,
  filter = "all",
  currentMedia = "",
  ready = false;
const project = () => state.project,
  clips = () => project()?.clips ?? [],
  clip = () => clips().find((c) => c.id === selected);
const clock = (sec) =>
  new Date(Math.max(0, sec) * 1000).toISOString().slice(11, 19);
const accepted = () => clips().filter((c) => c.status === "approved").length;
async function api(path, data) {
  const opts =
    data === undefined
      ? {}
      : {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-VODCut": "local" },
          body: JSON.stringify(data),
        };
  const response = await fetch("/api/" + path, opts);
  if (!response.ok) {
    const problem = await response.json().catch(() => ({}));
    throw Error(
      typeof problem.detail === "string" ? problem.detail : response.statusText,
    );
  }
  return response.json();
}
function notice(message) {
  $("notice").hidden = !message;
  $("notice").textContent = message;
}
function localize() {
  document.documentElement.lang = lang === "pt" ? "pt-BR" : "en";
  $("lang").textContent = lang === "pt" ? "EN" : "PT";
  document
    .querySelectorAll("[data-t]")
    .forEach((e) => (e.textContent = t(e.dataset.t)));
  document
    .querySelectorAll("[data-icon]")
    .forEach((e) => (e.innerHTML = icon(e.dataset.icon)));
}
function waveform() {
  const node = $("wave"),
    w = node.clientWidth,
    h = node.clientHeight,
    ratio = window.devicePixelRatio || 1;
  if (!w) return;
  node.width = w * ratio;
  node.height = h * ratio;
  const palette = getComputedStyle(document.documentElement);
  const ctx = node.getContext("2d");
  ctx.scale(ratio, ratio);
  const curve = project()?.curve || [],
    duration = project()?.meta.duration || 1,
    c = clip();
  if (c) {
    ctx.fillStyle = palette.getPropertyValue("--wave-fill").trim();
    ctx.fillRect(
      (w * c.start) / duration,
      0,
      (w * (c.end - c.start)) / duration,
      h,
    );
  }
  for (let x = 0; x < w; x += 4) {
    const v =
      curve[Math.min(curve.length - 1, Math.floor((x / w) * curve.length))] ||
      0;
    ctx.fillStyle = curve.length
      ? palette.getPropertyValue("--accent").trim()
      : "#46454b";
    const bar = Math.max(2, v * (h - 4));
    ctx.fillRect(x, (h - bar) / 2, 2, bar);
  }
  if (c) {
    ctx.fillStyle = palette.getPropertyValue("--accent").trim();
    ctx.fillRect((w * c.peak) / duration, 0, 1, h);
  }
}
function mediaURL(c, kind) {
  return "/api/media/" + project().project + "/" + c.id + "/" + kind;
}
function draw() {
  const p = project(),
    c = clip(),
    busy = state.job.busy;
  document.body.classList.toggle("welcome", !p);
  $("start").disabled = busy || (!ready && !state.defaults.top_n);
  $("reviewHint").textContent = accepted()
    ? accepted() === 1
      ? t("readyOne")
      : accepted() + " " + t("exportReady")
    : t("awaitApproval");
  $("import").disabled = busy;
  $("export").disabled = busy || !accepted();
  $("source").textContent = p ? p.meta.path : t("none");
  $("format").textContent = p ? p.meta.width + " × " + p.meta.height : "—";
  $("count").textContent = clips().length;
  $("total").textContent = p ? clock(p.meta.duration) : "--:--:--";
  $("timecode").textContent = c
    ? clock(c.start + ($("video").currentTime || 0))
    : "00:00:00";
  $("tabs").innerHTML = ["all", "approved", "rejected"]
    .map(
      (k) =>
        '<button class="' +
        (filter === k ? "active" : "") +
        '" data-filter="' +
        k +
        '">' +
        t(k) +
        "</button>",
    )
    .join("");
  const list = clips().filter((c) => filter === "all" || c.status === filter);
  $("cards").innerHTML = list
    .map(
      (x) =>
        '<button class="card ' +
        (x.id === selected ? "active" : "") +
        '" data-id="' +
        x.id +
        '"><img alt="" src="' +
        mediaURL(x, "thumb") +
        '"><span class="info"><strong>' +
        esc(x.name) +
        "</strong><small>" +
        clock(x.start) +
        " – " +
        clock(x.end) +
        " · " +
        t(x.status) +
        '</small></span><span class="score">' +
        Math.round(x.score * 100) +
        "</span></button>",
    )
    .join("");
  if (!list.length) {
    const title =
      filter !== "all" ? "emptyFilter" : p ? "noHits" : "emptyQueue";
    $("cards").innerHTML =
      '<div class="empty"><i>' +
      icon("stack") +
      "</i><h3>" +
      t(title) +
      "</h3><p>" +
      (filter !== "all" ? "" : t(p ? "noHitsSub" : "emptyQueueSub")) +
      "</p></div>";
  }
  $("emptyScreen").hidden = !!c;
  $("video").hidden = !c;
  $("emptyScreen").querySelector("h3").textContent = t(
    p ? "selectClip" : "emptyMonitor",
  );
  $("emptyScreen").querySelector("p").textContent = p
    ? ""
    : t("emptyMonitorSub");
  const url = c
    ? mediaURL(c, "preview") + "?range=" + c.start + "-" + c.end
    : "";
  if (url !== currentMedia) {
    currentMedia = url;
    if (url) {
      $("video").src = url;
      $("format").textContent = t("preparing");
    } else {
      $("video").removeAttribute("src");
      $("video").load();
    }
  }
  $("clipid").textContent = c ? "#" + c.id.slice(1) : "—";
  if (c) {
    $("editor").innerHTML =
      '<p class="trimHelp">' +
      t("editHint") +
      '</p><form id="editform"><div class="editgrid">' +
      [
        ["name", "text", c.name],
        ["start", "number", c.start],
        ["end", "number", c.end],
      ]
        .map(
          ([k, type, value]) =>
            "<label><span>" +
            t(k) +
            '</span><input id="edit-' +
            k +
            '" type="' +
            type +
            '" ' +
            (type === "number" ? 'step="any" min="0"' : 'maxlength="160"') +
            ' required value="' +
            esc(value) +
            '"></label>',
        )
        .join("") +
      '</div><div class="reviewline"><span class="mono">' +
      t("peak") +
      " " +
      clock(c.peak) +
      " · " +
      t("score") +
      " " +
      Math.round(c.score * 100) +
      "/100</span>" +
      [
        ["save", c.status],
        ["reject", "rejected"],
        ["approve", "approved"],
      ]
        .map(
          ([k, status]) =>
            "<button " +
            (busy ? "disabled " : "") +
            'class="' +
            (k === "approve" ? "primary" : "") +
            '" data-status="' +
            status +
            '">' +
            t(k) +
            "</button>",
        )
        .join("") +
      '</div></form><div class="evidence"><h3>' +
      t("evidenceTitle") +
      "</h3><strong>" +
      esc(c.reason[lang]) +
      "</strong><p>" +
      (c.context ? t("ctx") : "") +
      '</p><div class="chips">' +
      Object.entries(c.breakdown)
        .map(
          ([k, v]) => "<span>" + t(k) + " " + Math.round(v * 100) + "</span>",
        )
        .join("") +
      "</div><strong>" +
      t("transcript") +
      "</strong><p>" +
      esc(c.transcript || t("noWords")) +
      "</p></div>";
    $("editform").onsubmit = async (e) => {
      e.preventDefault();
      try {
        await api("clip/" + c.id, {
          project: p.project,
          run: p.run,
          name: $("edit-name").value,
          start: Number($("edit-start").value),
          end: Number($("edit-end").value),
          status: e.submitter?.dataset.status ?? c.status,
        });
        await refresh();
        notice(t("saved"));
      } catch (err) {
        notice(err.message);
      }
    };
  } else {
    $("editor").innerHTML = "<p>" + t("choose") + "</p>";
  }
  waveform();
}
function jobStatus() {
  const job = state.job;
  $("jobs").hidden = !job.busy;
  if (job.busy) {
    $("jobs").innerHTML =
      "<h2>" +
      t(job.steps.export !== undefined ? "packingTitle" : "analysisTitle") +
      "</h2><p>" +
      t("waitHint") +
      '</p><div class="jobline">' +
      (Object.entries(job.steps)
        .map(
          ([k, v]) =>
            "<span>" +
            t(k) +
            " " +
            Math.round(v * 100) +
            '% <progress max="1" value="' +
            v +
            '"></progress></span>',
        )
        .join("") || t("loading")) +
      '<button id="stop">' +
      t("cancel") +
      "</button></div>";
    $("stop").onclick = () => api("cancel", {}).catch((e) => notice(e.message));
  } else if (job.error) notice(job.error);
  else if (job.result) {
    notice(t("done") + " · " + job.result);
    const b = document.createElement("button");
    b.textContent = t("open");
    b.onclick = () => api("open-export", {}).catch((e) => notice(e.message));
    $("notice").append(b);
  }
}
async function refresh() {
  const previous = project()?.project,
    prevRun = project()?.run;
  state = await api("state");
  if (previous !== project()?.project || prevRun !== project()?.run)
    filter = "all";
  const shown = clips().filter((c) => filter === "all" || c.status === filter);
  if (!shown.some((x) => x.id === selected)) selected = shown[0]?.id ?? null;
  jobStatus();
  draw();
  ready = true;
}
$("lang").onclick = () => {
  lang = lang === "pt" ? "en" : "pt";
  localStorage.setItem("vodcut-lang", lang);
  localize();
  draw();
  jobStatus();
};
$("tabs").onclick = (e) => {
  const b = e.target.closest("button");
  if (b) {
    filter = b.dataset.filter;
    selected =
      clips().find((c) => filter === "all" || c.status === filter)?.id ?? null;
    draw();
  }
};
$("cards").onclick = (e) => {
  const b = e.target.closest("button[data-id]");
  if (b) {
    selected = b.dataset.id;
    draw();
  }
};
$("video").onloadedmetadata = () => {
  const m = project()?.meta;
  if (m) $("format").textContent = m.width + " × " + m.height;
};
$("video").ontimeupdate = () => {
  if (clip())
    $("timecode").textContent = clock(clip().start + $("video").currentTime);
};
$("video").onerror = () => {
  if (currentMedia) notice(t("previewError"));
};
$("start").onclick = () => $("import").click();
$("import").onclick = () => {
  const d = state.defaults;
  $("modules").innerHTML = ["audio", "transcription", "visual", "judge"]
    .map(
      (k) =>
        '<label><input type="checkbox" id="opt-' +
        k +
        '" ' +
        (d[k] ? "checked" : "") +
        ">" +
        "<span><b>" +
        t(k) +
        "</b><small>" +
        t(k + "Help") +
        "</small></span></label>",
    )
    .join("");
  document.querySelector(".advanced").open = false;
  for (const k of ["top_n", "min_seconds", "max_seconds"]) $(k).value = d[k];
  $("setup").querySelector(".error").textContent = "";
  $("setup").showModal();
};
document.querySelectorAll("[data-pick]").forEach(
  (b) =>
    (b.onclick = async () => {
      try {
        b.disabled = true;
        const kind = b.dataset.pick;
        const result = await api("pick/" + kind, {});
        if (result.path)
          $(kind === "chat" ? "chat" : "path").value = result.path;
      } catch (err) {
        $("setup").querySelector(".error").textContent = err.message;
      } finally {
        b.disabled = false;
      }
    }),
);
document
  .querySelectorAll(".close")
  .forEach((b) => (b.onclick = () => b.closest("dialog").close()));
$("setupform").onsubmit = async (e) => {
  e.preventDefault();
  try {
    const data = {
      path: $("path").value.trim().replace(/^"|"$/g, ""),
      chat: $("chat").value.trim().replace(/^"|"$/g, ""),
    };
    for (const k of ["top_n", "min_seconds", "max_seconds"])
      data[k] = Number($(k).value);
    for (const k of ["audio", "transcription", "visual", "judge"])
      data[k] = $("opt-" + k).checked;
    if (data.max_seconds < data.min_seconds) throw Error(t("durationError"));
    await api("analyze", data);
    $("setup").close();
    notice("");
    await refresh();
  } catch (err) {
    $("setup").querySelector(".error").textContent = err.message;
  }
};
$("export").onclick = () => {
  $("summary").textContent = t("approved") + " · " + accepted();
  $("delivery").querySelector(".error").textContent = "";
  updateFormat();
  $("delivery").showModal();
};
$("deliveryform").onsubmit = async (e) => {
  e.preventDefault();
  try {
    const formats = [
      ...$("deliveryform").querySelectorAll("input:checked"),
    ].map((x) => x.value);
    if (!formats.length) throw Error(t("formatError"));
    await api("export", {
      project: project().project,
      run: project().run,
      formats,
      vertical: $("vertical").value === "1",
    });
    $("delivery").close();
    notice("");
    await refresh();
  } catch (err) {
    $("delivery").querySelector(".error").textContent = err.message;
  }
};
localize();
new ResizeObserver(waveform).observe($("wave"));
refresh().catch((e) => notice(e.message));
let polling = false;
setInterval(async () => {
  if ((state.job.busy || !ready) && !polling) {
    polling = true;
    try {
      await refresh();
    } catch (e) {
      notice(e.message);
    } finally {
      polling = false;
    }
  }
}, 900);

function updateFormat() {
  $("vertical").disabled = !document.querySelector(
    '#delivery input[value="xml"]',
  ).checked;
}
$("deliveryform").addEventListener("change", updateFormat);
