class AppConfig {
  final int port;
  final String databaseUrl;
  final String env;

  AppConfig({
    this.port = 3000,
    this.databaseUrl = 'sqlite://:memory:',
    this.env = 'development',
  });

  factory AppConfig.fromEnv(Map<String, String> env) {
    return AppConfig(
      port: int.tryParse(env['PORT'] ?? '3000') ?? 3000,
      databaseUrl: env['DATABASE_URL'] ?? 'sqlite://:memory:',
      env: env['NODE_ENV'] ?? 'development',
    );
  }
}

AppConfig? _config;

AppConfig loadConfig({Map<String, String>? env}) {
  _config ??= AppConfig.fromEnv(env ?? const {});
  return _config!;
}

void resetConfig() {
  _config = null;
}
