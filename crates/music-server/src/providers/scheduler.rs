//! VOS provider adapter. Identity is forwarded by the authenticated loopback
//! gateway and held only in this account's private process, never on disk.
use super::openrouter::{CapabilityCatalog, CatalogModel, OpenRouterRequest};
use anyhow::{bail, Context, Result};
use axum::{extract::Request, middleware::Next, response::Response};
use music_core::Capability;
use serde_json::Value;
use std::sync::RwLock;

static TOKEN: RwLock<Option<String>> = RwLock::new(None);
pub fn enabled() -> bool {
    std::env::var("YUE_VOS").as_deref() == Ok("1")
}
pub fn token() -> Option<String> {
    TOKEN.read().ok()?.clone()
}
pub fn base() -> String {
    std::env::var("YUE_SCHEDULER_GATEWAY_URL")
        .unwrap_or_else(|_| "http://v-scheduler:8080".into())
        .trim_end_matches('/')
        .to_owned()
}
pub fn inference_base() -> String {
    format!("{}/v1", base())
}
pub async fn remember_identity(request: Request, next: Next) -> Response {
    if enabled() {
        if let Some(value) = request
            .headers()
            .get("X-VOS-Provider-Authorization")
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.strip_prefix("Bearer "))
        {
            if let Ok(mut stored) = TOKEN.write() {
                *stored = Some(value.to_owned());
            }
        }
    }
    if enabled()
        && request.method() == axum::http::Method::POST
        && [
            "/v1/music/jobs",
            "/v1/music/replay",
            "/v1/scores",
            "/v1/transcriptions",
        ]
        .contains(&request.uri().path())
        && std::env::var("YUE_COMPUTE_ADMITTED").as_deref() != Ok("1")
    {
        use axum::response::IntoResponse;
        return (
            axum::http::StatusCode::SERVICE_UNAVAILABLE,
            axum::Json(serde_json::json!({"error":"V-Scheduler compute admission required"})),
        )
            .into_response();
    }
    next.run(request).await
}
pub fn parse_catalog(body: Value) -> Result<CapabilityCatalog> {
    let rows = body["models"]
        .as_array()
        .context("V-Scheduler returned no model directory")?;
    let mut models = Vec::new();
    for row in rows {
        let Some(id) = row["qualified"].as_str().filter(|s| !s.is_empty()) else {
            continue;
        };
        let has = |key: &str, values: &[&str]| {
            row[key].as_array().is_some_and(|list| {
                list.iter()
                    .any(|v| values.contains(&v.as_str().unwrap_or_default()))
            })
        };
        let mut capabilities = Vec::new();
        if has("endpoints", &["chat_completions"]) && has("capabilities", &["chat", "completion"]) {
            capabilities.push(Capability::PromptEnhancement);
        }
        if has("endpoints", &["images/generations", "images_generations"])
            && has("capabilities", &["image_generation"])
        {
            capabilities.push(Capability::CoverArt);
        }
        if has(
            "endpoints",
            &["audio/transcriptions", "audio_transcriptions"],
        ) && has("capabilities", &["transcription", "asr", "speech_to_text"])
        {
            capabilities.push(Capability::SpeechToText);
        }
        if capabilities.is_empty() {
            continue;
        }
        models.push(CatalogModel {
            id: id.into(),
            name: id.into(),
            description: None,
            context_length: row["context_length"].as_u64(),
            modality: None,
            input_modalities: vec![],
            output_modalities: vec![],
            supported_parameters: vec![],
            pricing: None,
            capabilities,
            defaults: Default::default(),
            reasoning: None,
        });
    }
    models.sort_by(|a, b| a.id.cmp(&b.id));
    Ok(CapabilityCatalog { models })
}
pub async fn catalog() -> Result<CapabilityCatalog> {
    let token = token().context("VOS session required for V-Scheduler")?;
    let body = crate::net::client()
        .get(format!("{}/api/v1/models", base()))
        .bearer_auth(token)
        .send()
        .await?
        .error_for_status()?
        .json()
        .await?;
    parse_catalog(body)
}
pub async fn send(request: &OpenRouterRequest) -> Result<reqwest::Response> {
    let token = token().context("VOS session required for V-Scheduler")?;
    let mut call = crate::net::client()
        .post(format!("{}{}", inference_base(), request.path))
        .bearer_auth(token);
    if request.path == "/audio/transcriptions" {
        use base64::Engine as _;
        let body = &request.body;
        let data = body["input_audio"]["data"]
            .as_str()
            .context("Audio data missing")?;
        let format = body["input_audio"]["format"].as_str().unwrap_or("wav");
        if !["wav", "mp3", "flac", "ogg", "m4a", "webm", "mp4"].contains(&format) {
            bail!("Unsupported audio format");
        }
        let audio = base64::engine::general_purpose::STANDARD.decode(data)?;
        let mut form = reqwest::multipart::Form::new()
            .text(
                "model",
                body["model"].as_str().unwrap_or_default().to_owned(),
            )
            .part(
                "file",
                reqwest::multipart::Part::bytes(audio).file_name(format!("audio.{format}")),
            );
        for name in ["language", "response_format"] {
            if let Some(v) = body[name].as_str() {
                form = form.text(name, v.to_owned());
            }
        }
        if let Some(values) = body["timestamp_granularities"].as_array() {
            for value in values {
                if let Some(v) = value.as_str() {
                    form = form.text("timestamp_granularities[]", v.to_owned());
                }
            }
        }
        call = call.multipart(form);
    } else {
        let mut body = request.body.clone();
        if request.path == "/images/generations" {
            if let Some(object) = body.as_object_mut() {
                object.remove("size");
            }
            body["response_format"] = Value::String("b64_json".into());
        }
        call = call.json(&body);
    }
    Ok(call.send().await?)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    #[test]
    fn discover_only_explicit_supported_contracts_and_preserve_qualified_ids() {
        let c = parse_catalog(json!({"models":[
            {"qualified":"team/chat","capabilities":["chat"],"endpoints":["chat_completions"]},
            {"qualified":"team/image","capabilities":["image_generation"],"endpoints":["images/generations"]},
            {"qualified":"team/asr","capabilities":["asr"],"endpoints":["audio/transcriptions"]},
            {"qualified":"team/embedding","capabilities":["embedding"],"endpoints":["embeddings"]},
            {"qualified":"team/unknown","capabilities":["chat"]}
        ]})).unwrap();
        assert_eq!(c.models.len(), 3);
        assert!(c
            .selected(Capability::PromptEnhancement, "team/chat")
            .is_ok());
        assert!(c.selected(Capability::CoverArt, "team/chat").is_err());
        assert!(c.selected(Capability::SpeechToText, "team/asr").is_ok());
        assert!(c.selected(Capability::PromptEnhancement, "chat").is_err());
    }
}

/// Normalize the synchronous OpenAI image contract for the existing cover
/// editor. Provider credentials stay in Scheduler; a user bearer is attached
/// only to Scheduler's own origin, never an external image CDN.
pub async fn normalize_images(body: &mut Value) -> Result<()> {
    use base64::Engine as _;
    let Some(images) = body.get_mut("data").and_then(Value::as_array_mut) else {
        bail!("V-Scheduler returned no synchronous image data");
    };
    for image in images {
        if image["b64_json"].as_str().is_some() {
            continue;
        }
        let raw = image["url"]
            .as_str()
            .context("Image response has neither URL nor base64 data")?
            .to_owned();
        let gateway = reqwest::Url::parse(&base())?;
        let url = if raw.starts_with('/') {
            gateway.join(&raw)?
        } else {
            reqwest::Url::parse(&raw)?
        };
        if !["http", "https"].contains(&url.scheme()) {
            bail!("Unsupported image URL scheme");
        }
        let mut call = crate::net::client()
            .get(url.clone())
            .timeout(std::time::Duration::from_secs(30));
        if url.origin() == gateway.origin() {
            call = call.bearer_auth(token().context("VOS session required")?);
        }
        let mut response = call.send().await?.error_for_status()?;
        if response
            .content_length()
            .is_some_and(|n| n > 20 * 1024 * 1024)
        {
            bail!("Cover image exceeds 20 MiB");
        }
        let media = response
            .headers()
            .get("Content-Type")
            .and_then(|v| v.to_str().ok())
            .unwrap_or("image/png")
            .split(';')
            .next()
            .unwrap_or("image/png")
            .to_owned();
        if !["image/png", "image/jpeg", "image/webp", "image/gif"].contains(&media.as_str()) {
            bail!("V-Scheduler image URL returned an unsupported media type");
        }
        let mut bytes = Vec::new();
        while let Some(chunk) = response.chunk().await? {
            if bytes.len() + chunk.len() > 20 * 1024 * 1024 {
                bail!("Cover image exceeds 20 MiB")
            };
            bytes.extend_from_slice(&chunk);
        }
        image["b64_json"] = Value::String(base64::engine::general_purpose::STANDARD.encode(bytes));
        image["media_type"] = Value::String(media);
    }
    Ok(())
}
