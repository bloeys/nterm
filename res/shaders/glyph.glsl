//shader:vertex
#version 410

//aVertPos must be in the range [0,1]
layout(location=0) in vec3 aVertPos;

//Instanced
layout(location=1) in vec2 aUV0;
layout(location=2) in vec4 aVertColor;
layout(location=3) in vec3 aModelPos;
layout(location=4) in vec2 aModelScale;

out vec2 v2fUV0;
out vec4 v2fColor;
out vec3 v2fFragPos;

uniform mat4 projViewMat;
uniform vec2 modelSize;
uniform vec2 sizeUV;

void main()
{
    mat4 modelMat = mat4(
        aModelScale.x,  0.0,            0.0,            0.0, 
        0.0,            aModelScale.y,  0.0,            0.0, 
        0.0,            0.0,            1.0,            0.0, 
        aModelPos.x,    aModelPos.y,    aModelPos.z,    1.0
    );

    v2fUV0 = aUV0 + aVertPos.xy * sizeUV;
    v2fColor = aVertColor;

    gl_Position = projViewMat * modelMat * vec4(aVertPos, 1.0);
}

//shader:fragment
#version 410

in vec2 v2fUV0;
in vec4 v2fColor;

out vec4 fragColor;

uniform sampler2D diffTex;

void main()
{
    vec4 texColor = texture(diffTex, v2fUV0);
    // if (texColor.r == 0)
    // {
    //     fragColor = vec4(0,1,0,0.25);
    // }
    // else
    {
        fragColor = vec4(v2fColor.rgb, texColor.r*v2fColor.a);
    }
} 
