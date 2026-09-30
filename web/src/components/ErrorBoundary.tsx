import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';
import { Button, Result, Space } from 'antd';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('ErrorBoundary caught:', error, errorInfo);
  }

  handleReset = () => {
    this.setState({ hasError: false, error: undefined });
  };

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      const isChunkError =
        this.state.error?.message?.includes('dynamically imported module') ||
        this.state.error?.message?.includes('Loading chunk') ||
        this.state.error?.message?.includes('Importing a module script failed');
      return (
        <Result
          status="error"
          title="页面渲染出错"
          subTitle={
            isChunkError
              ? '页面资源已更新（服务重新部署过），刷新后即可恢复'
              : this.state.error?.message || '发生了未知错误'
          }
          extra={
            <Space>
              <Button type="primary" onClick={this.handleReload}>
                刷新页面
              </Button>
              <Button onClick={this.handleReset}>重试</Button>
            </Space>
          }
        />
      );
    }
    return this.props.children;
  }
}
