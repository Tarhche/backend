<script setup>
const videoUrl ='http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ElephantsDream.mp4'
</script>
<template>
  <div class="container">
    <div class="row">
        <div class="col-md-8 mx-auto " :style="{height: '300px'}">
          <videojs :video="videoUrl"  />
        </div>
    </div>
  </div>
</template>

<style scoped>

</style>