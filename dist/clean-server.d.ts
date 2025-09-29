/**
 * Clean Architecture WebSocket Server
 *
 * Follows clean architecture principles:
 * - Domain layer: Pure business logic, no dependencies
 * - Application layer: Use cases and services
 * - Infrastructure layer: External concerns (NATS, WebSocket, persistence)
 * - Presentation layer: Controllers and HTTP routes
 */
export declare class CleanArchitectureServer {
    private readonly app;
    private readonly server;
    private wss;
    private readonly clientRepository;
    private webSocketRepository;
    private natsSubscriber;
    private connectionService;
    private messageService;
    private webSocketController;
    private healthController;
    initialize(): Promise<void>;
    private setupDependencyInjection;
    private setupInfrastructure;
    private setupHTTPServer;
    private setupWebSocketServer;
    private setupPresentationLayer;
    private startBackgroundTasks;
    shutdown(): Promise<void>;
}
//# sourceMappingURL=clean-server.d.ts.map