import { Card } from "@heroui/react";

export default function StorageDBPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-default-900">DB Storage</h1>
        <p className="mt-1 text-sm text-default-500">DB 管理入口已预留。</p>
      </div>

      <Card className="border border-default-200/70 bg-background/70 shadow-sm">
        <Card.Header>
          <Card.Title className="text-base">Worker DB</Card.Title>
          <Card.Description>当前版本暂不提供 DB 可视化管理。</Card.Description>
        </Card.Header>
        <Card.Content>
          <div className="rounded-md border border-dashed border-default-300 px-4 py-12 text-center text-sm text-default-500">
            后续可在这里接入 Worker DB 表结构、查询和数据维护能力。
          </div>
        </Card.Content>
      </Card>
    </div>
  );
}
