use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use tokio::time::{interval, Duration};
use hyper_util::client::legacy::connect::HttpConnector;
use hyper_util::rt::TokioExecutor;
use crate::config::HealthCheckConfig;

#[derive(Clone)]
pub struct HealthChecker {
    statuses: Arc<RwLock<HashMap<String, bool>>>,
}

impl HealthChecker {
    pub fn new() -> Self {
        HealthChecker {
            statuses: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    pub fn spawn_health_check(
        &self,
        upstream_url: String,
        config: HealthCheckConfig,
        mut shutdown_rx: tokio::sync::broadcast::Receiver<()>,
    ) {
        let statuses = Arc::clone(&self.statuses);
        let upstream_key = upstream_url.clone();

        tokio::spawn(async move {
            let mut interval_timer = interval(Duration::from_millis(config.interval_ms));
            let connector = HttpConnector::new();
            let client: hyper_util::client::legacy::Client<HttpConnector, axum::body::Body> =
                hyper_util::client::legacy::Client::builder(TokioExecutor::new())
                .build(connector);

            loop {
                tokio::select! {
                    _ = interval_timer.tick() => {
                        let health_url = format!("{}{}", upstream_url, config.path);

                        let is_healthy = match hyper::Uri::try_from(health_url.as_str()) {
                            Ok(uri) => {
                                let req = hyper::Request::builder()
                                    .method("GET")
                                    .uri(uri)
                                    .body(axum::body::Body::empty())
                                    .unwrap_or_else(|_| {
                                        hyper::Request::new(axum::body::Body::empty())
                                    });

                                match client.request(req).await {
                                    Ok(resp) => {
                                        let status_code: hyper::http::StatusCode = resp.status();
                                        status_code.is_success()
                                    }
                                    Err(_) => false,
                                }
                            }
                            Err(_) => false,
                        };

                        let mut statuses = statuses.write().await;
                        statuses.insert(upstream_key.clone(), is_healthy);
                    }
                    _ = shutdown_rx.recv() => {
                        break;
                    }
                }
            }
        });
    }

    pub async fn is_healthy(&self, upstream_url: &str) -> bool {
        let statuses = self.statuses.read().await;
        statuses.get(upstream_url).copied().unwrap_or(true)
    }

    pub async fn get_all_statuses(&self) -> HashMap<String, bool> {
        let statuses = self.statuses.read().await;
        statuses.clone()
    }

    pub async fn set_status(&self, upstream_url: String, healthy: bool) {
        let mut statuses = self.statuses.write().await;
        statuses.insert(upstream_url, healthy);
    }
}

impl Default for HealthChecker {
    fn default() -> Self {
        Self::new()
    }
}
