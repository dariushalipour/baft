use crate::billing::domain::{BillingError, Order, OrderRepository, OrderStatus};

pub struct CreateOrderInput {
    pub user_id: String,
    pub items: Vec<OrderItemInput>,
}

pub struct OrderItemInput {
    pub product_id: String,
    pub quantity: i32,
}

pub struct CreateOrderService {
    repo: Box<dyn OrderRepository>,
}

impl CreateOrderService {
    pub fn new(repo: Box<dyn OrderRepository>) -> Self {
        Self { repo }
    }

    pub fn execute(&self, input: CreateOrderInput) -> Result<Order, BillingError> {
        if input.user_id.is_empty() {
            return Err(BillingError::validation("user_id", "required"));
        }
        if input.items.is_empty() {
            return Err(BillingError::validation("items", "at least one item required"));
        }
        for (i, item) in input.items.iter().enumerate() {
            if item.product_id.is_empty() {
                return Err(BillingError::validation(
                    &format!("items[{}].product_id", i),
                    "required",
                ));
            }
            if item.quantity <= 0 {
                return Err(BillingError::validation(
                    &format!("items[{}].quantity", i),
                    "must be positive",
                ));
            }
        }

        let mut order = Order {
            id: format!("order-{}", 0),
            user_id: input.user_id,
            items: input.items.into_iter().map(|item| crate::billing::domain::OrderItem {
                product_id: item.product_id,
                quantity: item.quantity,
                unit_price_cents: 0,
            }).collect(),
            status: OrderStatus::Pending,
            total_cents: 0,
            created_at: std::time::SystemTime::now(),
            updated_at: std::time::SystemTime::now(),
        };

        self.repo.save(&mut order)?;
        Ok(order)
    }
}
