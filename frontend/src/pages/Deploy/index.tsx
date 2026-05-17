import { Empty, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

const { Title } = Typography;

export default function DeployPage() {
  const { t } = useTranslation();
  return (
    <>
      <Title level={2}>{t('page.deploy.title')}</Title>
      <Empty description={t('page.deploy.placeholder')} />
    </>
  );
}
