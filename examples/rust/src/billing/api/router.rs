use crate::billing::service::create_order::CreateOrderInput;
use crate::billing::service::cancel_order::CancelOrderService;
use crate::shared::logger::{ConsoleLogger, Logger};

pub struct HttpRequest {
    pub method: String,
    pub path: String,
    pub body: String,
}

pub struct HttpResponse {
    pub status: u16,
    pub body: String,
}

pub struct Router {
    logger: ConsoleLogger,
}

impl Router {
    pub fn new() -> Self {
        Self {
            logger: ConsoleLogger,
        }
    }

    pub fn route(&self, req: HttpRequest) -> HttpResponse {
        match (req.method.as_str(), req.path.as_str()) {
            ("POST", "/orders") => self.handle_create_order(req),
            ("POST", path) if path.starts_with("/orders/") && path.ends_with("/cancel") => {
                self.handle_cancel_order(req)
            }
            _ => HttpResponse {
                status: 404,
                body: serde_json::to_string_pretty(&json!({"error": "not found"})).unwrap(),
            },
        }
    }

    fn handle_create_order(&self, _req: HttpRequest) -> HttpResponse {
        HttpResponse {
            status: 201,
            body: serde_json::to_string_pretty(&json!({"message": "created"})).unwrap(),
        }
    }

    fn handle_cancel_order(&self, _req: HttpRequest) -> HttpResponse {
        HttpResponse {
            status: 204,
            body: String::new(),
        }
    }
}
