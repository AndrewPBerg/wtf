export type ToolCallEvent = {
  toolName?: string;
  input: { command?: string };
};

export type ToolCallResult = { block: true; reason: string } | undefined;

type ToolCallHandler = (event: ToolCallEvent) => ToolCallResult | Promise<ToolCallResult>;

class MockEventBus extends Map<string, ToolCallHandler[]> {}

export class MockPi {
  events = new MockEventBus();

  on(event: string, handler: ToolCallHandler) {
    const handlers = this.events.get(event) ?? [];
    handlers.push(handler);
    this.events.set(event, handlers);
  }
}

export type ExtensionAPI = MockPi;

export function isToolCallEventType(toolName: string, event: ToolCallEvent) {
  return event.toolName === toolName;
}

export function createMockPi() {
  return new MockPi();
}
