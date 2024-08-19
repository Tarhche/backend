<template>
  <vue-plyr :options="options" >
    <video controls
           playsinline
           :data-poster="poster">
      <source
          size="1080p"
          :src="video"
          type="video/mp4">
      <!--<source size="576"
              src="https://cdn.plyr.io/static/demo/View_From_A_Blue_Moon_Trailer-576p.mp4"
              type="video/mp4">
      <source size="720"
              src="https://cdn.plyr.io/static/demo/View_From_A_Blue_Moon_Trailer-720p.mp4"
              type="video/mp4">
      <track default
             kind="captions"
             label="English captions"
             src="https://cdn.plyr.io/static/demo/View_From_A_Blue_Moon_Trailer-HD.en.vtt"
             srclang="en">
      <track kind="captions"
             label="Légendes en français"
             src="https://cdn.plyr.io/static/demo/View_From_A_Blue_Moon_Trailer-HD.fr.vtt"
             srclang="fr"> -->
      <a download=""
         :href="video">
        Download
      </a>
    </video>
  </vue-plyr>
</template>

<script setup lang="ts">
import VuePlyr from '@skjnldsv/vue-plyr'
const {video, poster} = defineProps({
  video: String,
  poster: String,
})
const options = {quality: {default: '1080p'}}
</script>
