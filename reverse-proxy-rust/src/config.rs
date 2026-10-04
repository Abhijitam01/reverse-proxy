use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct RouteMatch {
    pub host: Option<String>,
    pub path_prefix: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct HealthCheckConfig {
    pub path: String,
    pub interval_ms: u64,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct RouteConfig {
    #[serde(rename = "match")]
    pub match_: RouteMatch,
    pub upstream: String,
    pub strip_prefix: Option<bool>,
    pub add_request_headers: Option<HashMap<String, String>>,
    pub remove_request_headers: Option<Vec<String>>,
    pub add_response_headers: Option<HashMap<String, String>>,
    pub remove_response_headers: Option<Vec<String>>,
    pub health_check: Option<HealthCheckConfig>,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct ProxyConfig {
    pub listen_addr: String,
    pub listen_port: u16,
    pub routes: Vec<RouteConfig>,
}

impl Default for ProxyConfig {
    fn default() -> Self {
        ProxyConfig {
            listen_addr: "127.0.0.1".to_string(),
            listen_port: 3000,
            routes: Vec::new(),
        }
    }
}
