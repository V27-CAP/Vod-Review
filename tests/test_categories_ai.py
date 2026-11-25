import http.server
import json
import tempfile
import threading
import unittest
from pathlib import Path

from vodcut.classify import classify_clip, format_post, normalize_category, normalize_hashtags
from vodcut.core import config
from vodcut.judge import AIJudge, rerank


class CategoriesAndAITests(unittest.TestCase):
    def test_offline_classification_all_four_categories(self):
        funny = classify_clip(
            dict(
                start=10,
                end=30,
                transcript="kkkkk meu deus que piada engraçada, tô rindo muito!",
                breakdown={"chat_laugh": 0.9, "vocal": 0.8},
                signals=["chat_laugh", "vocal"],
            )
        )
        self.assertEqual(funny["category"], "funny")
        self.assertTrue(funny["description"])
        self.assertTrue(funny["context_note"])
        self.assertTrue(all(t.startswith("#") for t in funny["hashtags"]))

        intense = classify_clip(
            dict(
                start=10,
                end=28,
                transcript="CORRE! CUIDADO! CARACA QUE CLUTCH INSANO!!",
                breakdown={"loudness": 0.95, "change": 0.9, "motion": 0.8},
                signals=["loudness", "change"],
            )
        )
        self.assertEqual(intense["category"], "intense")

        cute = classify_clip(
            dict(
                start=10,
                end=25,
                transcript="Aww que gatinho mais fofo e lindo, muito obrigado pelo carinho!",
                breakdown={"loudness": 0.2, "facecam": 0.7},
                signals=["facecam"],
            )
        )
        self.assertEqual(cute["category"], "cute")

        story = classify_clip(
            dict(
                start=10,
                end=55,
                context=True,
                transcript=(
                    "Deixa eu contar uma história que aconteceu uma vez quando eu era criança, "
                    "nunca esqueci daquele dia na minha vida porque mudou tudo para sempre."
                ),
                breakdown={"loudness": 0.25},
                signals=["transcript"],
            )
        )
        self.assertEqual(story["category"], "story")

    def test_normalization_and_post_formatting(self):
        self.assertEqual(normalize_category("História marcante"), "story")
        self.assertEqual(normalize_category("Engraçado"), "funny")
        self.assertEqual(normalize_category("Fofo"), "cute")
        self.assertEqual(normalize_category("Intenso"), "intense")
        tags = normalize_hashtags("live, #shorts  #Live  engraçado")
        self.assertEqual(tags, ["#live", "#shorts", "#engraçado"])

        post = format_post(
            dict(
                name="Clutch impossível",
                description="Sobreviveu com 1 de HP!",
                context_note="Final da partida ranqueada.",
                hashtags=["#live", "#clutch"],
            )
        )
        self.assertIn("Clutch impossível", post)
        self.assertIn("Sobreviveu com 1 de HP!", post)
        self.assertIn("Contexto: Final da partida ranqueada.", post)
        self.assertIn("#live #clutch", post)

    def test_ai_provider_validation(self):
        cfg = config()["judge"]
        with self.assertRaises(ValueError):
            AIJudge(cfg | {"provider": "lmstudio", "endpoint": "https://remote.example/v1/chat/completions"})
        with self.assertRaises(ValueError):
            AIJudge(cfg | {"provider": "openai", "api_key": "", "key_env": "NONEXISTENT_KEY_XYZ"})
        with self.assertRaises(ValueError):
            AIJudge(cfg | {"provider": "gemini", "endpoint": "http://remote.example/v1beta", "api_key": "k"})

        ollama = AIJudge(cfg | {"provider": "ollama", "endpoint": "http://127.0.0.1:11434"})
        self.assertEqual(ollama.endpoint, "http://127.0.0.1:11434/v1/chat/completions")

        gemini = AIJudge(cfg | {"provider": "gemini", "model": "gemini-2.5-flash", "api_key": "test-key"})
        self.assertTrue(gemini.endpoint.endswith("/models/gemini-2.5-flash:generateContent"))

    def test_openai_and_gemini_vision_requests_and_rerank(self):
        received = []

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                length = int(self.headers.get("Content-Length", "0"))
                body = json.loads(self.rfile.read(length))
                received.append((self.path, dict(self.headers), body))
                ai_json = json.dumps(
                    {
                        "funny": 0.9,
                        "surprising": 0.7,
                        "understandable": 0.95,
                        "reason_pt": "Reação hilária na facecam.",
                        "reason_en": "Hilarious facecam reaction.",
                        "category": "funny",
                        "categories": ["funny", "intense"],
                        "title": "Reação impagável na live",
                        "context_note": "O streamer caiu na gargalhada após o bug.",
                        "description": "Esse bug pegou todo mundo de surpresa!",
                        "hashtags": ["#live", "#bug", "#engracado"],
                    }
                )
                if ":generateContent" in self.path:
                    resp = {"candidates": [{"content": {"parts": [{"text": ai_json}]}}]}
                else:
                    resp = {"choices": [{"message": {"content": ai_json}}]}
                data = json.dumps(resp).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        port = server.server_address[1]
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory() as tmp:
                tmp_path = Path(tmp)
                fake_jpg = tmp_path / "crop.jpg"
                fake_jpg.write_bytes(b"\xff\xd8\xff\xe0FAKEJPEG")

                # 1. Local OpenAI-compatible (Ollama/LM Studio) with vision image_url
                cfg_local = config()["judge"] | {
                    "provider": "ollama",
                    "endpoint": f"http://127.0.0.1:{port}",
                    "model": "llava",
                    "vision": True,
                }
                local_judge = AIJudge(cfg_local)
                out_local = local_judge.rate("Que bug foi esse?!", image_path=fake_jpg)
                self.assertEqual(out_local["category"], "funny")
                self.assertEqual(out_local["title"], "Reação impagável na live")
                self.assertIn("#engracado", out_local["hashtags"])
                _, _, local_body = received[-1]
                user_content = local_body["messages"][1]["content"]
                self.assertIsInstance(user_content, list)
                self.assertEqual(user_content[1]["type"], "image_url")
                self.assertTrue(user_content[1]["image_url"]["url"].startswith("data:image/jpeg;base64,"))

                # 2. Gemini with inline_data vision crop and x-goog-api-key header
                cfg_gemini = config()["judge"] | {
                    "provider": "gemini",
                    "endpoint": f"http://127.0.0.1:{port}/v1beta",
                    "model": "gemini-2.5-flash",
                    "api_key": "secret-gemini-key",
                    "vision": True,
                }
                gemini_judge = AIJudge(cfg_gemini)
                out_gemini = gemini_judge.rate("Que bug foi esse?!", image_path=fake_jpg)
                self.assertEqual(out_gemini["category"], "funny")
                path_hit, headers_hit, gemini_body = received[-1]
                self.assertIn(":generateContent", path_hit)
                self.assertEqual(headers_hit.get("x-goog-api-key") or headers_hit.get("X-Goog-Api-Key"), "secret-gemini-key")
                parts = gemini_body["contents"][0]["parts"]
                self.assertEqual(parts[1]["inline_data"]["mime_type"], "image/jpeg")

                # 3. rerank updates clip metadata cleanly
                clip = dict(
                    id="p10",
                    name="Highlight 01",
                    start=5,
                    end=20,
                    peak=10.5,
                    score=0.4,
                    transcript="Que bug foi esse?!",
                    breakdown={},
                )
                ranked = rerank([clip], cfg_local, tmp_path / "cache", lambda *_: None, local_judge)
                self.assertEqual(ranked[0]["name"], "Reação impagável na live")
                self.assertEqual(ranked[0]["category"], "funny")
                self.assertEqual(ranked[0]["categories"], ["funny", "intense"])
                self.assertEqual(ranked[0]["context_note"], "O streamer caiu na gargalhada após o bug.")
                self.assertEqual(ranked[0]["description"], "Esse bug pegou todo mundo de surpresa!")
                self.assertEqual(ranked[0]["hashtags"], ["#live", "#bug", "#engracado"])
        finally:
            server.shutdown()
            server.server_close()
