mod auth;
mod billing;
mod shared;

use billing::api::router::Router;

fn main() {
    let router = Router::new();
    println!("server listening on port 8080");

    let req = billing::api::HttpRequest {
        method: "GET".to_string(),
        path: "/health".to_string(),
        body: String::new(),
    };
    let _resp = router.route(req);
}
