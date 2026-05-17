import { Empty, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

const { Title } = Typography;

export default function WorkloadsPage() {
  const { t } = useTranslation();
  return (
    <>
      <Title level={2}>{t('page.workloads.title')}</Title>
      <Empty description={t('page.workloads.placeholder')} />
    </>
  );
}
