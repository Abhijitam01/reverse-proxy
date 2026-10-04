use axum::{
    http::StatusCode,
    response::{IntoResponse, Response},
    Json,
};
use serde_json::json;

#[derive(Debug)]
pub enum AppError {
    ConfigError(String),
    ProxyError(String),
    InvalidRoute(String),
    UpstreamError(String),
    UnhealthyUpstream(String),
    NotFound(String),
}

impl std::fmt::Display for AppError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AppError::ConfigError(msg) => write!(f, "Configuration error: {}", msg),
            AppError::ProxyError(msg) => write!(f, "Proxy error: {}", msg),
            AppError::InvalidRoute(msg) => write!(f, "Invalid route: {}", msg),
            AppError::UpstreamError(msg) => write!(f, "Upstream error: {}", msg),
            AppError::UnhealthyUpstream(msg) => write!(f, "Unhealthy upstream: {}", msg),
            AppError::NotFound(msg) => write!(f, "Not found: {}", msg),
        }
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        let (status, error_message) = match &self {
            AppError::ConfigError(msg) => (StatusCode::INTERNAL_SERVER_ERROR, msg.clone()),
            AppError::ProxyError(msg) => (StatusCode::BAD_GATEWAY, msg.clone()),
            AppError::InvalidRoute(msg) => (StatusCode::BAD_REQUEST, msg.clone()),
            AppError::UpstreamError(msg) => (StatusCode::BAD_GATEWAY, msg.clone()),
            AppError::UnhealthyUpstream(_) => (
                StatusCode::BAD_GATEWAY,
                "Upstream service is unhealthy".to_string(),
            ),
            AppError::NotFound(msg) => (StatusCode::NOT_FOUND, msg.clone()),
        };

        let body = Json(json!({
            "error": error_message,
        }));

        (status, body).into_response()
    }
}
