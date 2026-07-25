import { Button, Result, Typography } from 'antd';
import { useNavigate } from 'react-router-dom';
import { HomeOutlined } from '@ant-design/icons';

export default function NotFound() {
  const navigate = useNavigate();

  return (
    <Result
      status="404"
      title="页面不存在"
      subTitle="您访问的页面不存在或尚未实现。"
      extra={
        <Button type="primary" icon={<HomeOutlined />} onClick={() => navigate('/')}>
          返回首页
        </Button>
      }
    />
  );
}
