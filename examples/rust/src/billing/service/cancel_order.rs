use crate::billing::domain::{BillingError, OrderRepository, OrderStatus};

pub struct CancelOrderService {
    repo: Box<dyn OrderRepository>,
}

impl CancelOrderService {
    pub fn new(repo: Box<dyn OrderRepository>) -> Self {
        Self { repo }
    }

    pub fn execute(&self, order_id: &str) -> Result<(), BillingError> {
        let mut order = self.repo.find_by_id(order_id)?
            .ok_or_else(|| BillingError::not_found(&format!("Order {}", order_id)))?;

        if order.status != OrderStatus::Pending {
            return Err(BillingError::conflict("only pending orders can be cancelled"));
        }

        order.status = OrderStatus::Cancelled;
        self.repo.save(&mut order)
    }
}
