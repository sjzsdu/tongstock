import { useEffect } from 'react';
import { Card, Button } from 'antd';

export function TestComponent() {
  useEffect(() => {}, []);
  return <Card><Button>Test</Button></Card>;
}
