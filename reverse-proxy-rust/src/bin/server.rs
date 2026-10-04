use reverse_proxy_rust::{
    config::ProxyConfig,
    routes::build_router,
    state::AppState,
};
use std::sync::Arc;
use tokio::net::TcpListener;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config = ProxyConfig::default();

    let state = Arc::new(AppState::new(config.routes));

    let router = build_router(state);

    let listener = TcpListener::bind(format!("{}:{}", config.listen_addr, config.listen_port))
        .await
        .map_err(|e| format!("Failed to bind to address: {}", e))?;

    println!(
        "Reverse proxy listening on {}:{}",
        config.listen_addr, config.listen_port
    );

    axum::serve(listener, router)
        .await
        .map_err(|e| format!("Server error: {}", e).into())
}
