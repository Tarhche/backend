import {Title, Text, Button, Container, Group} from "@mantine/core";
import {IconRefresh, IconMoodSadDizzy} from "@tabler/icons-react";
import classes from "./error.module.css";

type Props = {
  onReset: () => void;
};

export function Error({onReset}: Props) {
  return (
    <div className={classes.root}>
      <Container>
        <div className={classes.label}>
          <IconMoodSadDizzy size={150} />
        </div>
        <Title className={classes.title}>خطایی رخ داد</Title>
        <Text size="lg" ta="center" className={classes.description}>
          اتفاقی غیر منتظره رخ داد، لطفا دوباره تلاش کنید و اگر همچنان مشکل
          داشتید با ما در ارتباط باشید
        </Text>
        <Group justify="center">
          <Button
            variant="subtle"
            size="md"
            leftSection={<IconRefresh />}
            onClick={onReset}
          >
            تلاش مجدد
          </Button>
        </Group>
      </Container>
    </div>
  );
}
