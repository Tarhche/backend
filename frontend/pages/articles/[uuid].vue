<template>
    <div>article's uuid: {{ $route.params.uuid }}</div>
</template>

<script setup>
    const route = useRoute();
    console.log(route.params.uuid);
</script>