use serde::{Deserialize, Serialize};
use std::time::SystemTime;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub enum OrderStatus {
    Pending,
    Confirmed,
    Shipped,
    Delivered,
    Cancelled,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrderItem {
    pub product_id: String,
    pub quantity: i32,
    pub unit_price_cents: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Order {
    pub id: String,
    pub user_id: String,
    pub items: Vec<OrderItem>,
    pub status: OrderStatus,
    pub total_cents: u32,
    pub created_at: SystemTime,
    pub updated_at: SystemTime,
}

pub trait OrderRepository {
    fn find_by_id(&self, id: &str) -> Result<Option<Order>, BillingError>;
    fn save(&self, order: &mut Order) -> Result<(), BillingError>;
    fn list_by_user(&self, user_id: &str, limit: usize) -> Result<Vec<Order>, BillingError>;
}
