
import 'core/config.dart';
import 'core/logger.dart';

void main() async {
  final config = loadConfig();
  final logger = Logger();

  logger.info('Application starting', context: {'version': '1.0.0'});

  // In a real app, these would be wired up with dependency injection
  // final createOrder = CreateOrderUseCase(/* repo */, logger);
  // final cancelOrder = CancelOrderUseCase(/* repo */, logger);
  // final router = ApiRouter(
  //   createOrder: createOrder,
  //   cancelOrder: cancelOrder,
  //   logger: logger,
  // );

  logger.info('Server listening on port ${config.port}');
  print('Server listening on port ${config.port}');
}
