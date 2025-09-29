import { Request, Response } from 'express';
import { ConnectionService } from '../../application/services/ConnectionService';
import { MessageService } from '../../application/services/MessageService';
import { NatsEventSubscriber } from '../../infrastructure/nats/NatsEventSubscriber';
export declare class HealthController {
    private readonly connectionService;
    private readonly messageService;
    private readonly natsSubscriber;
    private readonly startTime;
    constructor(connectionService: ConnectionService, messageService: MessageService, natsSubscriber: NatsEventSubscriber);
    getHealth(req: Request, res: Response): Promise<void>;
    getStats(req: Request, res: Response): Promise<void>;
    getMetrics(req: Request, res: Response): Promise<void>;
    resetMetrics(req: Request, res: Response): Promise<void>;
}
//# sourceMappingURL=HealthController.d.ts.map