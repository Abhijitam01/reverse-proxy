use std::sync::Arc;
use tokio::sync::RwLock;
use crate::config::RouteConfig;
use crate::health::HealthChecker;

#[derive(Clone)]
pub struct AppState {
    pub routes: Arc<RwLock<Vec<RouteConfig>>>,
    pub health_checker: HealthChecker,
    pub shutdown_tx: Arc<tokio::sync::broadcast::Sender<()>>,
}

impl AppState {
    pub fn new(initial_routes: Vec<RouteConfig>) -> Self {
        let (tx, _) = tokio::sync::broadcast::channel(100);
        AppState {
            routes: Arc::new(RwLock::new(initial_routes)),
            health_checker: HealthChecker::new(),
            shutdown_tx: Arc::new(tx),
        }
    }

    pub async fn add_route(&self, route: RouteConfig) {
        let mut routes = self.routes.write().await;
        routes.push(route);
    }

    pub async fn remove_route(&self, index: usize) -> bool {
        let mut routes = self.routes.write().await;
        if index < routes.len() {
            routes.remove(index);
            true
        } else {
            false
        }
    }

    pub async fn get_routes(&self) -> Vec<RouteConfig> {
        let routes = self.routes.read().await;
        routes.clone()
    }
}
