<template>
  <div>
    <Navbar></Navbar>
    <slot/>
    <Footer></Footer>
  </div>
</template>

<script setup>
useHead({
  titleTemplate: (title) => title ? ` طرح چه | ${title} ` : "طرح چه"
})
</script>
