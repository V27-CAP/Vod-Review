"""Offline moment categorization, context notes, descriptions, and social post formatting."""

from __future__ import annotations

import re
import unicodedata

CATEGORIES = ("funny", "intense", "cute", "story")

CATEGORY_LABELS = {
    "funny": ("Engraçado", "Funny"),
    "intense": ("Intenso", "Intense"),
    "cute": ("Fofo", "Cute"),
    "story": ("História marcante", "Memorable story"),
}

_ALIASES = {
    "funny": "funny",
    "engracado": "funny",
    "comedia": "funny",
    "humor": "funny",
    "risada": "funny",
    "meme": "funny",
    "intense": "intense",
    "intenso": "intense",
    "hype": "intense",
    "clutch": "intense",
    "acao": "intense",
    "action": "intense",
    "susto": "intense",
    "cute": "cute",
    "fofo": "cute",
    "fofa": "cute",
    "fofura": "cute",
    "wholesome": "cute",
    "carinhoso": "cute",
    "story": "story",
    "historia": "story",
    "historia marcante": "story",
    "relato": "story",
    "desabafo": "story",
    "memorable story": "story",
    "storytime": "story",
}

_PATTERNS = {
    "funny": re.compile(
        r"k{3,}|(?:ha){2,}|(?:rs){2,}|(?:ja){3,}|www+|笑|草|"
        r"\[(?:laughter|laughing|snorts?)\]|"
        r"blew up|i panicked|i forgot|not a compliment|that'?s what they call me|i'?m goated|i didn'?t notice|"
        r"\b(?:kekw|lulw?|lma+o|lmfao|lol|rofl|piada|engraçad\w*|rindo|morri|zueira|troll\w*|meme|"
        r"funny|hilarious|joke|laugh\w*|cursed|bruh|shut up|excuse me|goddamn|bullshit|clown|idiot|fail\w*)\b",
        re.IGNORECASE,
    ),
    "intense": re.compile(
        r"!{2,}|\?!|\?\?|"
        r"\[(?:screaming|screams?|gasps?|panting)\]|"
        r"oh my god|oh god|what the|what the hell|fucking hell|no,? no|no{3,}|a+h{3,}|we did it|i'?m dead|i'?m hit|i'?m panick\w*|too high|mission failed|time'?s up|jump ?scare|"
        r"\b(?:clutch|boss|morreu|caraca|caralho|meu deus|não acredito|nao acredito|socorro|insan\w*|absurd\w*|"
        r"corre|cuidado|susto|grit\w*|gg|ganhei|perdi|nervos\w*|tens\w*|crazy|omg|wtf|no way|let'?s go|"
        r"pog(?:champ)?|holy|fuck\w*|shit\w*|warning|missile|dodge|crash\w*|scary|terrifying|run|help)\b",
        re.IGNORECASE,
    ),
    "cute": re.compile(
        r"<3|♥|❤|🥰|🥹|"
        r"\b(?:fof\w*|lind\w*|amor\w*|coração|coracao|obrigad\w*|carinh\w*|gatinh\w*|gato|cachorr\w*|pet|dog\w*|cat|"
        r"aww+|cute|wholesome|sweet\w*|lovely|adorable|precious|blessed|comfy|cozy|headpats?|heart|blush\w*|"
        r"bichinh\w*|nen[eê]m|querid\w*)\b",
        re.IGNORECASE,
    ),
    "story": re.compile(
        r"\[crying\]|i'?m (?:not )?crying|tearing up|made me cry|thank you (?:so much|everyone|guys)|"
        r"real talk|my whole life|actually cried|"
        r"\b(?:uma vez|lembro|lembra|quando eu|história|historia|aconteceu|dia que|contar|época|epoca|"
        r"nunca esqueci|sabe quando|na minha vida|infância|infancia|verdade que|remember|one time|story|"
        r"happened|when i was|tell you|true story|growing up|back when|sincere|emotional|finale|ending|"
        r"proud|journey|appreciate|crying)\b",
        re.IGNORECASE,
    ),
}

_DEFAULT_TAGS = {
    "funny": ["#live", "#clipes", "#shorts", "#engracado", "#risada"],
    "intense": ["#live", "#clipes", "#shorts", "#intenso", "#clutch"],
    "cute": ["#live", "#clipes", "#shorts", "#fofo", "#wholesome"],
    "story": ["#live", "#clipes", "#shorts", "#historia", "#storytime"],
}

_DEFAULT_TAGS_EN = {
    "funny": ["#vtuber", "#clips", "#funny", "#highlights", "#gaming"],
    "intense": ["#vtuber", "#clips", "#intense", "#highlights", "#gaming"],
    "cute": ["#vtuber", "#clips", "#wholesome", "#cute", "#highlights"],
    "story": ["#vtuber", "#clips", "#emotional", "#highlights", "#gaming"],
}

_SIGNAL_NOTES = {
    "loudness": ("pico de volume", "volume spike"),
    "change": ("mudança súbita de energia", "sudden energy jump"),
    "vocal": ("reação vocal forte", "strong vocal reaction"),
    "transcript": ("reação na fala", "spoken reaction"),
    "chat_rate": ("chat acelerado", "fast chat spike"),
    "chat_laugh": ("risadas no chat", "chat laughter"),
    "chat_users": ("alta participação do chat", "high chat participation"),
    "motion": ("ação intensa na tela", "intense on-screen action"),
    "facecam": ("expressão marcante na facecam", "strong facecam/avatar reaction"),
}


def _strip_accents(text: str) -> str:
    norm = unicodedata.normalize("NFKD", text)
    return "".join(ch for ch in norm if not unicodedata.combining(ch))


def normalize_category(value: str | None) -> str:
    if not value:
        return "funny"
    cleaned = re.sub(r"[^a-z0-9 ]+", " ", _strip_accents(str(value).lower())).strip()
    cleaned = re.sub(r"\s+", " ", cleaned)
    if cleaned in _ALIASES:
        return _ALIASES[cleaned]
    for alias, cat in _ALIASES.items():
        if alias in cleaned:
            return cat
    return "funny"


def category_label(category: str, lang: str = "pt") -> str:
    cat = normalize_category(category)
    idx = 1 if lang == "en" else 0
    return CATEGORY_LABELS[cat][idx]


def normalize_hashtags(tags: list | str | None, fallback_category: str = "funny", lang: str = "pt") -> list[str]:
    raw_items: list[str] = []
    if isinstance(tags, str):
        raw_items = re.split(r"[\s,;]+", tags.strip())
    elif isinstance(tags, (list, tuple)):
        for item in tags:
            raw_items.extend(re.split(r"[\s,;]+", str(item).strip()))
    seen: set[str] = set()
    result: list[str] = []
    for item in raw_items:
        token = re.sub(r"[^\w\u00C0-\u024F]", "", item.lstrip("#"), flags=re.UNICODE)
        if not token:
            continue
        tag = "#" + token
        key = tag.lower()
        if key not in seen:
            seen.add(key)
            result.append(tag)
    if not result:
        defaults = _DEFAULT_TAGS_EN if lang == "en" else _DEFAULT_TAGS
        return list(defaults.get(normalize_category(fallback_category), defaults["funny"]))
    return result[:10]


def _quote_excerpt(text: str, limit: int = 120) -> str:
    cleaned = re.sub(r"\s+", " ", (text or "").strip())
    if len(cleaned) <= limit:
        return cleaned
    cut = cleaned[: limit - 1].rsplit(" ", 1)[0]
    return (cut or cleaned[: limit - 1]).rstrip(",.;:") + "…"


def classify_clip(clip: dict, lang: str = "en") -> dict:
    """Assign offline category, categories, context_note, description, and hashtags."""
    transcript = (clip.get("transcript") or "").strip()
    breakdown = clip.get("breakdown") or {}
    words = re.findall(r"\S+", transcript)
    word_count = len(words)
    duration = max(1.0, float(clip.get("end", 0)) - float(clip.get("start", 0)))
    is_en = lang == "en"

    scores = {
        "funny": 0.0,
        "intense": 0.0,
        "cute": 0.0,
        "story": 0.0,
    }

    for cat, pattern in _PATTERNS.items():
        hits = len(pattern.findall(transcript))
        if hits:
            scores[cat] += min(1.2, 0.45 * hits)

    scores["funny"] += (
        0.55 * float(breakdown.get("chat_laugh", 0))
        + 0.30 * float(breakdown.get("vocal", 0))
        + 0.20 * float(breakdown.get("facecam", 0))
    )
    scores["intense"] += (
        0.45 * float(breakdown.get("loudness", 0))
        + 0.35 * float(breakdown.get("change", 0))
        + 0.25 * float(breakdown.get("motion", 0))
        + 0.20 * float(breakdown.get("chat_rate", 0))
        + 0.15 * float(breakdown.get("facecam", 0))
    )
    # Soft/warm spoken moments with lower harsh loudness bias gently toward cute if cute cues exist
    if scores["cute"] > 0:
        scores["cute"] += 0.20 * (1.0 - float(breakdown.get("loudness", 0))) + 0.15 * float(
            breakdown.get("facecam", 0)
        )
    # Story bias: sustained speech density + spoken setup/follow-through
    speech_density = word_count / duration
    if word_count >= 24 or (word_count >= 16 and clip.get("context")):
        scores["story"] += min(0.75, 0.25 + 0.015 * word_count)
    if speech_density >= 1.5 and float(breakdown.get("loudness", 0)) < 0.7:
        scores["story"] += 0.15
    if clip.get("context"):
        scores["story"] += 0.12

    ranked = sorted(CATEGORIES, key=lambda c: (-scores[c], CATEGORIES.index(c)))
    primary = ranked[0]
    if scores[primary] <= 0.05:
        primary = "story" if word_count >= 20 else ("intense" if breakdown.get("loudness", 0) >= 0.4 else "funny")

    categories = [primary]
    for cat in ranked[1:]:
        if scores[cat] >= max(0.35, scores[primary] * 0.65):
            categories.append(cat)

    idx = 1 if is_en else 0
    cues = [_SIGNAL_NOTES[s][idx] for s in clip.get("signals", []) if s in _SIGNAL_NOTES]
    cue_text = ", ".join(cues) if cues else ("highlight detected in stream" if is_en else "destaque detectado na live")
    excerpt = _quote_excerpt(transcript, 110)

    label = category_label(primary, "en" if is_en else "pt")
    if is_en:
        if excerpt:
            context_note = f"{label} moment ({cue_text}): \"{excerpt}\""
        else:
            context_note = f"{label} moment marked by {cue_text}."
        openers = {
            "funny": "Couldn't stop laughing at this stream moment!",
            "intense": "What an intense stream moment!",
            "cute": "Such a wholesome stream moment!",
            "story": "This memorable moment had to make the edit!",
        }
        hashtags = list(_DEFAULT_TAGS_EN[primary])
        if breakdown.get("facecam", 0) >= 0.35 and "#reaction" not in hashtags:
            hashtags.append("#reaction")
        if clip.get("context") and "#stream" not in hashtags:
            hashtags.append("#stream")
    else:
        if excerpt:
            context_note = f"Momento {label.lower()} ({cue_text}): \"{excerpt}\""
        else:
            context_note = f"Momento {label.lower()} marcado por {cue_text}."
        openers = {
            "funny": "Não aguentei essa parte da live!",
            "intense": "Que momento insano na live!",
            "cute": "O momento mais fofo da live!",
            "story": "Essa história na live precisava virar corte!",
        }
        hashtags = list(_DEFAULT_TAGS[primary])
        if breakdown.get("facecam", 0) >= 0.35 and "#reacao" not in hashtags:
            hashtags.append("#reacao")
        if clip.get("context") and "#stream" not in hashtags:
            hashtags.append("#stream")

    if excerpt:
        description = f"{openers[primary]} \"{_quote_excerpt(transcript, 90)}\""
    else:
        description = openers[primary]

    hashtags = normalize_hashtags(hashtags, primary, lang="en" if is_en else "pt")

    return dict(
        category=primary,
        categories=categories,
        context_note=context_note,
        description=description,
        hashtags=hashtags,
    )


def format_post(clip: dict, lang: str = "pt") -> str:
    """Format a ready-to-copy social media post for a clip."""
    title = (clip.get("name") or "").strip()
    description = (clip.get("description") or "").strip()
    context_note = (clip.get("context_note") or "").strip()
    tags = normalize_hashtags(clip.get("hashtags"), clip.get("category", "funny"))
    blocks: list[str] = []
    if title:
        blocks.append(title)
    if description:
        blocks.append(description)
    if context_note:
        prefix = "Context: " if lang == "en" else "Contexto: "
        blocks.append(prefix + context_note)
    if tags:
        blocks.append(" ".join(tags))
    return "\n\n".join(blocks)
