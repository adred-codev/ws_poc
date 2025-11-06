import { Kafka, Producer, Message } from 'kafkajs';
import {
  EVENT_TYPE_TOPICS,
  TokenEvent,
  EventType,
} from './types/event-types.js';

export class RedpandaPublisher {
  private kafka: Kafka;
  private producer: Producer;
  private isConnected: boolean = false;

  constructor(brokers: string[]) {
    this.kafka = new Kafka({
      clientId: 'odin-publisher',
      brokers,
      retry: {
        initialRetryTime: 100,
        retries: 8,
      },
    });

    this.producer = this.kafka.producer({
      allowAutoTopicCreation: false,
      maxInFlightRequests: 5,
    });
  }

  async connect(): Promise<void> {
    if (this.isConnected) {
      return;
    }

    await this.producer.connect();
    this.isConnected = true;
    console.log('[RedpandaPublisher] Connected to Redpanda');
  }

  async disconnect(): Promise<void> {
    if (!this.isConnected) {
      return;
    }

    await this.producer.disconnect();
    this.isConnected = false;
    console.log('[RedpandaPublisher] Disconnected from Redpanda');
  }

  /**
   * Publish a single token event
   */
  async publishEvent(event: TokenEvent): Promise<void> {
    if (!this.isConnected) {
      throw new Error('Publisher not connected. Call connect() first.');
    }

    const topic = EVENT_TYPE_TOPICS[event.type];
    if (!topic) {
      throw new Error(`Unknown event type: ${event.type}`);
    }

    const message: Message = {
      key: event.tokenId, // Partition by token ID
      value: JSON.stringify({
        type: event.type,
        timestamp: event.timestamp,
        data: event.data,
      }),
      timestamp: event.timestamp.toString(),
    };

    await this.producer.send({
      topic,
      messages: [message],
    });
  }

  /**
   * Publish multiple events in a batch (more efficient)
   */
  async publishBatch(events: TokenEvent[]): Promise<void> {
    if (!this.isConnected) {
      throw new Error('Publisher not connected. Call connect() first.');
    }

    if (events.length === 0) {
      return;
    }

    // Group events by topic
    const eventsByTopic = new Map<string, Message[]>();

    for (const event of events) {
      const topic = EVENT_TYPE_TOPICS[event.type];
      if (!topic) {
        console.warn(
          `[RedpandaPublisher] Unknown event type: ${event.type}, skipping`
        );
        continue;
      }

      if (!eventsByTopic.has(topic)) {
        eventsByTopic.set(topic, []);
      }

      const message: Message = {
        key: event.tokenId,
        value: JSON.stringify({
          type: event.type,
          timestamp: event.timestamp,
          data: event.data,
        }),
        timestamp: event.timestamp.toString(),
      };

      eventsByTopic.get(topic)!.push(message);
    }

    // Send batches to each topic
    const sendPromises = Array.from(eventsByTopic.entries()).map(
      ([topic, messages]) =>
        this.producer.send({
          topic,
          messages,
        })
    );

    await Promise.all(sendPromises);
  }

  /**
   * Health check
   */
  isHealthy(): boolean {
    return this.isConnected;
  }
}
