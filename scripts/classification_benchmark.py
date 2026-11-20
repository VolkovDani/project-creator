"""Opt-in single-record SystemOne experiment, independent of the product pipeline."""
from datetime import date
from hashlib import sha256
from http.client import HTTPConnection
import json
import math
import re


MODEL = "laya:322m-multilingual-bf16"
RUNTIME_TEXT_LIMIT = 64 * 1024
TRAINING_CONTEXT = 1024
HEAD_LIMIT = 256
OPTION_LIMIT = 48
OLLAMA_VERSION = "0.40.16"
AUDITED_DIGEST = "c1242e4a83f0e1cb58f0f191a65bbc5d3306fae274d152b8229ec574999d465d"
AXES = ("semantic", "representation")
PROBE = "user_story_probe"
# All variants use the same contract codes. The second wording changes only the
# descriptions. Positional wire IDs keep option order stable in sorted JSON maps.
DESCRIPTIONS = {
    "business_requirement": ("Цель бизнеса", "Бизнес-цель"),
    "stakeholder_requirement": ("Нужда роли", "Запрос роли"),
    "functional_requirement": ("Функция", "Действие"),
    "non_functional_requirement": ("Качество", "Атрибут"),
    "transition_requirement": ("Переход", "Внедрение"),
    "business_rule": ("Правило", "Политика"),
    "constraint": ("Ограничение", "Рамки"),
    "statement": ("Утверждение", "Указание"),
    "user_story": ("Роль, нужда, цель", "Кто, что, зачем"),
    "use_case": ("Цель и потоки", "Пути актора"),
    "scenario": ("Ход событий", "Эпизод"),
    "acceptance_criterion": ("Условие приёмки", "Критерий"),
    "test_case": ("Шаги и ожидание", "Тест по шагам"),
    "other": ("Другое", "Иное"),
    "unknown": ("Неясно", "Неизвестно"),
    "need": ("Цель или нужда",),
    "system": ("Система или переход",),
    "policy": ("Правило или рамки",),
    "narrative": ("История или события",),
    "verification": ("Проверка результата",),
    "residual": ("Иное или неясное",),
    "not_user_story": ("Нет признаков истории",),
}
GROUPS = {
    "semantic": {
        "need": ("business_requirement", "stakeholder_requirement"),
        "system": ("functional_requirement", "non_functional_requirement", "transition_requirement"),
        "policy": ("business_rule", "constraint"), "residual": ("other", "unknown"),
    },
    "representation": {
        "statement": ("statement",), "narrative": ("user_story", "use_case", "scenario"),
        "verification": ("acceptance_criterion", "test_case"), "residual": ("other", "unknown"),
    },
}
SCHEMES = {
    "flat": {"method": "flat", "order": "contract", "wording": 0, "view": "full"},
    "flat_reverse": {"method": "flat", "order": "reverse", "wording": 0, "view": "full"},
    "flat_reword": {"method": "flat", "order": "contract", "wording": 1, "view": "full"},
    "hierarchical": {"method": "hierarchical", "order": "contract", "wording": 0, "view": "full"},
    "story_probe": {"method": "binary", "order": "contract", "wording": 0, "view": "full"},
    "flat_marker": {"method": "flat", "order": "contract", "wording": 0, "view": "marker"},
}
MARKER = re.compile(r"\b(?:дано|допустим|когда|если|предуслови[ея]|шаг(?:и|\s+\d+)|ожидаемый результат|как\s+[^\n,.]{1,50},?\s+я\s+хочу)\b", re.I)
INSTRUCTIONS = {
    "semantic": "Тип по смыслу?",
    "representation": "Форма?",
    PROBE: "Пользовательская история?",
}


def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()


def fingerprint(value):
    return sha256(encoded(value)).hexdigest()


def validate_corpus(corpus, contract):
    if not isinstance(corpus, dict) or corpus.get("status") != "reviewed" or corpus.get("language") != "ru":
        raise ValueError("A frozen, human-reviewed Russian corpus is required")
    if corpus.get("taxonomy") != contract["version"]:
        raise ValueError("Reviewed corpus taxonomy differs from the contract")
    if not isinstance(corpus.get("reviewer"), str) or not corpus["reviewer"].strip():
        raise ValueError("Missing human reviewer")
    try:
        date.fromisoformat(corpus.get("reviewed_at", ""))
    except (TypeError, ValueError) as exc:
        raise ValueError("Expected an ISO review date") from exc
    rows = corpus.get("examples")
    if not isinstance(rows, list) or not rows:
        raise ValueError("No reviewed examples")
    ids, texts, splits = set(), set(), set()
    for row in rows:
        required = ("id", "text", "source", "license", "split", *AXES)
        if not isinstance(row, dict) or "proposed" in row or not all(
            isinstance(row.get(k), str) and row[k].strip() for k in required
        ):
            raise ValueError("Every record requires reviewed labels, provenance, and a predefined split")
        normalized = " ".join(row["text"].casefold().split())
        if row["id"] in ids or normalized in texts:
            raise ValueError("Duplicate ID or text in reviewed corpus")
        ids.add(row["id"])
        texts.add(normalized)
        splits.add(row["split"])
        if any(row[axis] not in contract[axis] for axis in AXES):
            raise ValueError("Reviewed label outside classification contract")
    if splits != {"tuning", "holdout"}:
        raise ValueError("Supply both predefined tuning and holdout splits")
    for axis in AXES:
        supported = {label for group in GROUPS[axis].values() for label in group}
        if set(contract[axis]) - supported:
            raise ValueError("Unsupported taxonomy in hierarchical prompt")


def select_input(text, view, full_bytes, marker_bytes):
    size = len(encoded(text))
    start, end = 0, len(text)
    reason = None
    if view == "marker":
        match = MARKER.search(text)
        if match:
            start = match.start()
            low, high = start, end
            while low < high:
                mid = (low + high + 1) // 2
                if len(encoded(text[start:mid])) <= marker_bytes:
                    low = mid
                else:
                    high = mid - 1
            end = low
        else:
            reason = "no_marker"
    elif size > full_bytes:
        reason = "over_budget"
    if reason is None and any(token in text[start:end] for token in ("<bos>", "<eos>", "<mask>")):
        reason = "reserved_token"
    state = text[start:end] if reason is None else None
    return state, {
        "reason": reason, "start": start if state is not None else None,
        "end": end if state is not None else None, "original_chars": len(text),
        "original_bytes": len(text.encode()), "original_state_bytes": size,
        "input_bytes": len(state.encode()) if state is not None else 0,
        "state_bytes": len(encoded(state)) if state is not None else 0,
    }


def check_pin(tags, digest):
    match = [m for m in tags["models"] if m.get("name") == MODEL or m.get("model") == MODEL]
    if len(match) != 1 or match[0].get("remote_model") or match[0].get("remote_host"):
        raise ValueError("Pinned local Laya model unavailable")
    if match[0].get("digest", "").removeprefix("sha256:") != digest:
        raise ValueError("Local Laya digest mismatch")


def unit_score(value):
    return type(value) in (float, int) and math.isfinite(value) and 0 <= value <= 1


def decode(response, options):
    answers = response.get("answers", {})
    usage = response.get("usage")
    if response.get("model") != MODEL or set(answers) != {"classification"} or not isinstance(usage, dict):
        raise ValueError("Unexpected model, answers, or usage")
    if not all(type(usage.get(k)) is int and usage[k] >= 0 for k in ("input_tokens", "output_tokens")):
        raise ValueError("Invalid model token accounting")
    answer = answers["classification"]
    scores = answer.get("probabilities", {})
    if (answer.get("type") != "choice" or answer.get("choice") not in options
            or not unit_score(answer.get("confidence")) or set(scores) != set(options)
            or not all(unit_score(v) for v in scores.values()) or abs(sum(scores.values()) - 1) > 1e-6):
        raise ValueError("Invalid choice or option scores")
    return answer, usage


class RequestRejected(ValueError):
    def __init__(self, status):
        super().__init__(f"SystemOne rejected the request (HTTP {status})")
        self.status = status


class LocalLaya:
    """Direct IPv4 loopback HTTP connection without redirects or HTTP proxy settings."""

    def __init__(self, port, digest):
        self.digest = digest.removeprefix("sha256:")
        if self.digest != AUDITED_DIGEST or not 1 <= port <= 65535:
            raise ValueError("Provide the audited GGUF manifest SHA-256 and a valid loopback port")
        self.port = port

    def request(self, path, payload=None):
        conn = HTTPConnection("127.0.0.1", self.port, timeout=120)
        try:
            conn.request("GET" if payload is None else "POST", path,
                         body=encoded(payload) if payload is not None else None,
                         headers={"Content-Type": "application/json"})
            response = conn.getresponse()
            if path == "/v1/systemone" and response.status in (400, 413):
                raise RequestRejected(response.status)
            if response.status != 200:
                raise ValueError(f"Local Laya returned HTTP {response.status} at {path}")
            data = response.read(2**20 + 1)
            if len(data) > 2**20:
                raise ValueError("Oversize Laya response")
            return json.loads(data)
        finally:
            conn.close()

    def ask(self, state, question):
        check_pin(self.request("/api/tags"), self.digest)
        try:
            response = self.request("/v1/systemone", {
                "model": MODEL, "state": state, "questions": {"classification": question}, "keep_alive": "5m",
            })
        finally:
            check_pin(self.request("/api/tags"), self.digest)
        return decode(response, question["criteria"])


def run_experiments(corpus, contract, ask, full_bytes, marker_bytes):
    validate_corpus(corpus, contract)
    questions, schemes = {}, {}

    def choose(state, axis, labels, profile):
        order = list(labels)
        if profile["order"] == "reverse":
            order.reverse()
        ids = {chr(ord("a") + index): label for index, label in enumerate(order)}
        criteria = {key: DESCRIPTIONS[label][profile["wording"]] for key, label in ids.items()}
        question = {"type": "choice", "instructions": INSTRUCTIONS[axis], "criteria": criteria}
        # Upper bound for the pinned GGUF byte-level tokenizer and SystemOne template.
        option_sizes = [len(f" {key}: {desc}".encode()) for key, desc in criteria.items()]
        options = sum(n + 1 for n in option_sizes)  # one mask marker per option
        head = len(("choice question: " + question["instructions"]).encode()) + options
        if max(option_sizes) > OPTION_LIMIT or options > HEAD_LIMIT - 16 or head > HEAD_LIMIT:
            raise ValueError("Question exceeds the verified GGUF prompt-head budget")
        definition = {"wire": question, "labels": ids, "head_bounds": {
            "all_tokens_upper_bound": head, "longest_option_tokens_upper_bound": max(option_sizes)}}
        key = fingerprint(definition)
        questions[key] = definition
        try:
            answer, usage = ask(state, question)
        except RequestRejected as exc:
            return {"question": key, "reason": "runtime_rejected", "http_status": exc.status}
        label = ids[answer["choice"]]
        scores = {ids[k]: v for k, v in answer["probabilities"].items()}
        step = {"question": key, "label": label, "scores": scores,
                "confidence": answer["confidence"], "usage": usage}
        if usage["input_tokens"] > TRAINING_CONTEXT:
            step["reason"] = "outside_training_context"
        return step

    def predict(state, axis, profile, input_reason):
        steps = []
        if state is None:
            return {"label": None, "score": None, "steps": steps, "reason": input_reason}
        if profile["method"] == "hierarchical":
            groups = {group: [label for label in labels if label in contract[axis]]
                      for group, labels in GROUPS[axis].items()}
            first = choose(state, axis, [g for g, labels in groups.items() if labels], profile)
            steps.append(first)
            labels = groups[first["label"]] if "reason" not in first else []
        else:
            labels = ("user_story", "not_user_story") if axis == PROBE else contract[axis]
        if len(labels) > 1:
            steps.append(choose(state, axis, labels, profile))
        if "reason" in steps[-1]:
            return {"label": None, "score": None, "steps": steps, "reason": steps[-1]["reason"]}
        label = labels[0] if len(labels) == 1 else steps[-1]["label"]
        return {"label": label, "score": min(s["scores"][s["label"]] for s in steps),
                "steps": steps, "reason": None}

    for name, profile in SCHEMES.items():
        rows = []
        axes = (PROBE,) if profile["method"] == "binary" else AXES
        for example in corpus["examples"]:
            state, coverage = select_input(example["text"], profile["view"], full_bytes, marker_bytes)
            row = {"id": example["id"], "input": coverage}
            for axis in axes:
                row[axis] = predict(state, axis, profile, coverage["reason"])
            rows.append(row)
        schemes[name] = {"profile": profile, "predictions": rows}
    return {"questions": questions, "schemes": schemes}
